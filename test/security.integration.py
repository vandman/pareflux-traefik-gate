#!/usr/bin/env python3
###############################################################################
# Real Traefik AppSec regression checks
# FR: Vérifie la restitution des corps, les en-têtes forgés et le délai des clients lents.
# EN: Checks body replay, forged protocol headers and slow-client deadlines.
# Called by: GitLab integration and developers; Calls: loopback-only test stack.
###############################################################################
import http.client
import hashlib
import json
import os
import select
import socket
import time

port = int(os.environ.get('PAREFLUX_TEST_PORT', '18080'))
headers = {'Host': 'pareflux.localhost', 'Content-Type': 'application/json'}
# FR: Attend le routeur Docker du récepteur, pas seulement son conteneur démarré.
# EN: Waits for the receiver's Docker router, not merely its started container.
ready = False
for attempt in range(40):
    client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
    try:
        client.request('GET', '/uploads/fixture', headers=headers)
        response = client.getresponse()
        data = response.read()
        ready = response.status == 200 and response.getheader('Content-Type') == 'application/json' and json.loads(data).get('bytes') == 0
    except (OSError, ValueError, http.client.HTTPException):
        ready = False
    finally:
        client.close()
    if ready:
        break
    time.sleep(.25)
assert ready, 'Upload fixture route did not become ready'

payload = b'{"proof":"body-preserved","padding":"' + b'a' * 128 + b'"}'
for extra in [{}, {'Upgrade': 'websocket'}, {'Content-Type': 'application/grpc'}]:
    client = http.client.HTTPConnection('127.0.0.1', port, timeout=8)
    client.request('POST', '/', payload, {**headers, **extra})
    response = client.getresponse()
    body = response.read()
    assert response.status == 200 and payload in body, (response.status, body[:100])
    client.close()

client = http.client.HTTPConnection('127.0.0.1', port, timeout=8)
client.request('POST', '/', b'a' * ((1 << 20) + 1), headers)
response = client.getresponse()
assert response.status == 413, response.status
response.read()
client.close()

client = http.client.HTTPConnection('127.0.0.1', port, timeout=8)
upload = b'a' * ((1 << 20) + 1)
client.request('POST', '/uploads/fixture', upload, headers)
response = client.getresponse()
assert response.status == 200, response.status
assert response.getheader('X-Pareflux-Appsec-Inspection') == 'partial'
received = json.loads(response.read())
assert received == {'bytes': len(upload), 'sha256': hashlib.sha256(upload).hexdigest()}, received
client.close()

with socket.create_connection(('127.0.0.1', port), timeout=8) as client:
    client.sendall(b'POST / HTTP/1.1\r\nHost: pareflux.localhost\r\nContent-Type: application/json\r\nContent-Length: 1024\r\n\r\n{')
    start = time.monotonic()
    response = client.recv(4096)
    elapsed = time.monotonic() - start
    assert b' 408 ' in response and elapsed < 6, (response[:100], elapsed)
print('PASS: replay, forged streaming headers, oversized bodies, scoped upload exception and slow-client deadline')

# FR: Quatre uploads gardent une suite lente ; leur préfixe ne doit plus monopoliser le quota.
# EN: Four uploads retain a slow tail; their prefix must no longer monopolize quota.
slow = []
try:
    for attempt in range(4):
        client = socket.create_connection(('127.0.0.1', port), timeout=8)
        client.sendall(b'POST /uploads/fixture HTTP/1.1\r\nHost: pareflux.localhost\r\nContent-Type: application/octet-stream\r\nContent-Length: 2097152\r\n\r\n' + b'a' * ((1 << 20) + 1))
        slow.append(client)
        time.sleep(.1)
    for attempt in range(8):
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
        client.request('POST', '/', b'tiny', headers)
        response = client.getresponse()
        assert response.status == 200, ('slow-tail quota or memory retained', response.status)
        response.read(); client.close()
    for client in slow:
        response = client.recv(4096)
        assert response, 'Slow upload connection did not terminate at a finite deadline'
finally:
    for client in slow:
        client.close()

# FR: Une réponse que le client ne lit pas ne retient pas sa réservation d'inspection.
# EN: A response left unread by the client does not retain its inspection reservation.
slow = []
try:
    for attempt in range(4):
        client = socket.create_connection(('127.0.0.1', port), timeout=8)
        client.sendall(b'POST /uploads/slow-response HTTP/1.1\r\nHost: pareflux.localhost\r\nContent-Length: 4\r\n\r\ntiny')
        slow.append(client)
        time.sleep(.1)
    client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
    client.request('POST', '/', b'healthy', headers)
    response = client.getresponse()
    assert response.status == 200, ('slow response retained inspection quota', response.status)
    response.read(); client.close()
finally:
    for client in slow:
        client.close()
print('PASS: slow upload tails and unread backend responses release inspection resources')

# FR: Des uploads sans données saturent le quota corps sans bloquer les GET du navigateur.
# EN: Uploads with no data saturate the body quota without blocking browser GETs.
from concurrent.futures import ThreadPoolExecutor
slow = []
try:
    for attempt in range(4):
        client = socket.create_connection(('127.0.0.1', port), timeout=8)
        client.sendall(b'POST /uploads/fixture HTTP/1.1\r\nHost: pareflux.localhost\r\nContent-Length: 1048576\r\n\r\nx')
        slow.append(client)
    time.sleep(.15)
    # browser_get
    # FR: Une requête locale normale, sans corps, doit rester inspectée et autorisée.
    # EN: A normal local bodyless request must remain inspected and allowed.
    # Called by: executor; Calls: loopback test route.
    def browser_get(_):
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
        try:
            client.request('GET', '/', headers=headers)
            response = client.getresponse()
            response.read()
            return response.status
        finally:
            client.close()
    with ThreadPoolExecutor(max_workers=12) as pool:
        statuses = list(pool.map(browser_get, range(24)))
    assert statuses == [200] * 24, statuses
finally:
    for client in slow:
        client.close()
print('PASS: parallel browser GETs remain available with the body quota saturated')

# FR: Plus de 64 sources lentes ne doivent pas refuser le POST sain d'une autre source.
# EN: More than 64 slow sources must not reject a healthy POST from another source.
slow = []
try:
    started = time.monotonic()
    for index in range(96):
        client = socket.create_connection(('127.0.0.1', port), timeout=8)
        client.sendall(('POST /uploads/fixture HTTP/1.1\r\nHost: pareflux.localhost\r\n'
                        'Content-Length: 1048576\r\n'
                        f'X-Pareflux-Fixture-Client: 198.18.0.{index + 1}\r\n\r\nx').encode())
        slow.append(client)
    assert time.monotonic() - started < 2, 'Fixture failed to overlap the slow reads'
    time.sleep(.2)
    assert not select.select(slow, [], [], 0)[0], 'Slow readers were rejected before the inspection deadline'
    client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
    try:
        client.request('POST', '/uploads/fixture', b'healthy',
                       {**headers, 'X-Pareflux-Fixture-Client': '198.18.1.1'})
        response = client.getresponse()
        received = json.loads(response.read())
        assert response.status == 200 and received['bytes'] == 7, (response.status, received)
    finally:
        client.close()
    for client in slow:
        response = client.recv(4096)
        # FR: Les refus 408/503 restent bloquants ; le contrôle précédent exclut la saturation précoce.
        # EN: Both 408/503 remain blocking; the preceding check excludes early capacity rejection.
        assert b' 408 ' in response or b' 503 ' in response, ('Slow reader was not blocked', response[:100])
    assert time.monotonic() - started < 8, 'Slow readers exceeded finite fixture timeout'
finally:
    for client in slow:
        client.close()
client = http.client.HTTPConnection('127.0.0.1', port, timeout=3)
try:
    client.request('POST', '/uploads/fixture', b'recovered',
                   {**headers, 'X-Pareflux-Fixture-Client': '198.18.0.1'})
    response = client.getresponse()
    assert response.status == 200, ('Quota retained after slow reader cleanup', response.status)
    response.read()
finally:
    client.close()
print('PASS: 96 stalled sources leave healthy bodies available and release their reader quotas')
