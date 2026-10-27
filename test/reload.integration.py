#!/usr/bin/env python3
###############################################################################
# Real Traefik engine reload regression
# FR: Tourne les clés d'un faux flux local et vérifie l'arrêt des anciens workers.
# EN: Rotates local fixture stream keys and verifies obsolete workers stop.
# Called by: CI and developers; Calls: isolated Compose fixture, file provider.
###############################################################################
import http.client
import hashlib
import hmac
import secrets
import json
import os
from pathlib import Path
import subprocess
import time

root = Path(__file__).resolve().parents[1]
port = int(os.environ.get('PAREFLUX_TEST_PORT', '18080'))
project = os.environ.get('COMPOSE_PROJECT_NAME', 'pareflux-gate-test')
compose = [str(root/'scripts/ci-docker.sh'), 'compose', '-p', project, '-f', str(root/'test/docker-compose.yml')]

# get
# FR: Interroge uniquement le routeur local de la pile jetable.
# EN: Queries only the disposable stack's local router.
# Called by: checks below; Calls: loopback HTTP.
def get(host, path):
    client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
    try:
        client.request('GET', path, headers={'Host': host})
        response = client.getresponse()
        return response.status, response.read()
    finally:
        client.close()

# stats
# FR: Ne retourne que les compteurs des clés factices, sans clé de production.
# EN: Returns only fixture-key counters, with no production credentials.
# Called by: reload checks; Calls: fixture stats endpoint.
def stats():
    status, data = get('pareflux.localhost', '/uploads/_stats')
    assert status == 200, status
    return json.loads(data)

previous = None
for prefix in ['reload-a', 'reload-b', 'reload-c']:
    key = prefix + '-' + secrets.token_hex(4)
    options = {'enabled': True, 'logLevel': 'DEBUG', 'crowdsecMode': 'stream',
               'crowdsecLapiScheme': 'http', 'crowdsecLapiHost': 'upload-fixture:8080',
               'crowdsecLapiPath': '/', 'crowdsecLapiKey': key,
               'updateIntervalSeconds': 1, 'metricsUpdateIntervalSeconds': 0,
               'crowdsecAppsecEnabled': True, 'crowdsecAppsecHost': 'crowdsec:7422',
               'crowdsecAppsecKey': 'test-gate-key'}
    config = {'http': {'middlewares': {'reload': {'plugin': {'pareflux': options}}},
                      'routers': {'reload': {'entryPoints': ['web'], 'rule': 'Host(`reload.pareflux.localhost`)',
                                             'middlewares': ['reload'], 'service': 'reload'}},
                      'services': {'reload': {'loadBalancer': {'servers': [{'url': 'http://upload-fixture:8080'}]}}}}}
    discovery_key = 'fixture-discovery-key-at-least-32-bytes'
    menu_options = {**options, 'enabled': False, 'dashboardIntegration': True,
                    'dashboardHost': 'reload.pareflux.localhost', 'dashboardDiscoveryKey': discovery_key,
                    'dashboardRegistrationAllowed': True}
    config['http']['middlewares']['discovery'] = {'plugin': {'pareflux': menu_options}}
    config['http']['routers']['discovery'] = {'entryPoints': ['web'], 'rule': 'Host(`reload.pareflux.localhost`) && Path(`/.well-known/pareflux`)', 'middlewares': ['discovery'], 'service': 'reload'}
    subprocess.run(compose + ['exec', '-T', 'traefik', 'sh', '-c',
                   'cat > /run/fixture-dynamic/next.tmp && mv /run/fixture-dynamic/next.tmp /run/fixture-dynamic/state.yaml'],
                   input=json.dumps(config).encode(), check=True, capture_output=True)
    ready = False
    for attempt in range(50):
        time.sleep(.2)
        if stats().get(key, 0) > 0:
            status, _ = get('reload.pareflux.localhost', '/')
            if status == 200:
                ready = True
                break
    assert ready, ('Reload failed or invalid decision globally blocked visitors', key)
    # FR: Vérifie la découverte signée avec le protocole AppSec hérité.
    # EN: Verifies signed discovery with an inherited AppSec protocol.
    discovered = False
    for attempt in range(50):
        # FR: Une nouvelle annonce signée distingue le chargement du moteur de la publication du routeur.
        # EN: A fresh signed announcement separates engine loading from router publication.
        body = json.dumps({'control_url': 'https://console.fixture.example/', 'nonce': secrets.token_hex(16)}).encode()
        stamp = str(int(time.time()))
        signature = hmac.new(discovery_key.encode(), b'pareflux.discovery.v1\nrequest\n' + stamp.encode() + b'\n' + body, hashlib.sha256).hexdigest()
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=4)
        try:
            client.request('POST', '/.well-known/pareflux', body, {'Host': 'reload.pareflux.localhost', 'Content-Type': 'application/json', 'X-Pareflux-Time': stamp, 'X-Pareflux-Signature': signature})
            response = client.getresponse()
            data = response.read()
            assert response.status == 200, ('Signed discovery HTTP failure', response.status, data)
            stamp = response.getheader('X-Pareflux-Time')
            expected = hmac.new(discovery_key.encode(), b'pareflux.discovery.v1\nresponse\n' + stamp.encode() + b'\n' + data, hashlib.sha256).hexdigest()
            assert hmac.compare_digest(expected, response.getheader('X-Pareflux-Signature'))
            discovered = json.loads(data)['range_decisions']
        finally:
            client.close()
        if discovered:
            break
        time.sleep(.2)
    assert discovered, ('Exact engine discovery did not become ready', data)
    if previous:
        time.sleep(1)
        old = stats().get(previous, 0)
        time.sleep(3)
        assert stats().get(previous, 0) == old, ('Obsolete engine still polls after reload', previous)
    previous = key
print('PASS: real Traefik credential reloads stop obsolete workers, ignore unsupported decisions and sign exact-engine discovery')
