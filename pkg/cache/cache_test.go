// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux derived source file
// FR: Fichier dérivé du plugin CrowdSec Traefik v1.6.0 et adapté à PareFlux
//
//	sous Apache-2.0 sans dépendance GPL-3.0.
//
// EN: File derived from CrowdSec Traefik Plugin v1.6.0 and adapted for
//
//	PareFlux under Apache-2.0 without a GPL-3.0 dependency.
//
// Called by: PareFlux Gate runtime or tests
// Calls: package-specific collaborators
// /////////////////////////////////////////////////////////////////////////////
// Package cache implements utility routines for manipulating cache.
// It supports currently local file and redis cache.
package cache

import (
	"net"
	"testing"
	"time"

	logger "github.com/vandman/pareflux-traefik-gate/pkg/logger"
	simpleredis "github.com/vandman/pareflux-traefik-gate/pkg/simpleredis"
)

func Test_Get(t *testing.T) {
	IPInCache := "10.0.0.10"
	IPNotInCache := "10.0.0.20"
	client := &Client{cache: &localCache{}, log: logger.New("INFO", "")}
	client.Set(IPInCache, BannedValue, 10)
	type args struct {
		clientIP string
	}
	tests := []struct {
		name     string
		args     args
		want     string
		wantErr  bool
		valueErr string
	}{
		{name: "Fetch Known valid IP", args: args{clientIP: IPInCache}, want: BannedValue, wantErr: false, valueErr: ""},
		{name: "Fetch Unknown valid IP", args: args{clientIP: IPNotInCache}, want: "", wantErr: true, valueErr: CacheMiss},
		{name: "Fetch invalid value", args: args{clientIP: "test"}, want: "", wantErr: true, valueErr: CacheMiss},
		{name: "Fetch empty value", args: args{clientIP: ""}, want: "", wantErr: true, valueErr: CacheMiss},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.Get(tt.args.clientIP)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
				return
			}
			if tt.valueErr != "" && tt.valueErr != err.Error() {
				t.Errorf("Get() err = %v, want %v", err.Error(), tt.valueErr)
			}
		})
	}
}

func Test_Set(t *testing.T) {
	client := &Client{cache: &localCache{}, log: logger.New("INFO", "")}
	IPInCache := "10.0.0.11"
	type args struct {
		clientIP string
		value    string
		duration int64
	}

	tests := []struct {
		name     string
		args     args
		want     string
		wantErr  bool
		valueErr string
	}{
		{name: "Set valid IP in local cache for 0 sec", args: args{clientIP: IPInCache, value: BannedValue, duration: 0}, want: "", wantErr: true, valueErr: CacheMiss},
		{name: "Set valid IP in local cache for 10 sec", args: args{clientIP: IPInCache, value: BannedValue, duration: 10}, want: BannedValue, wantErr: false, valueErr: ""},
		{name: "Set valid IP in local cache for 10 sec", args: args{clientIP: IPInCache, value: NoBannedValue, duration: 10}, want: NoBannedValue, wantErr: false, valueErr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.Set(tt.args.clientIP, tt.args.value, tt.args.duration)
			got, err := client.Get(tt.args.clientIP)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Set() = %v, want %v", got, tt.want)
				return
			}
			if tt.valueErr != "" && tt.valueErr != err.Error() {
				t.Errorf("Set() err = %v, want %v", err.Error(), tt.valueErr)
			}
		})
	}
}

func Test_Delete(t *testing.T) {
	IPInCache := "10.0.0.12"
	IPNotInCache := "10.0.0.22"
	client := &Client{cache: &localCache{}, log: logger.New("INFO", "")}
	client.Set(IPInCache, BannedValue, 10)
	type args struct {
		clientIP string
	}

	tests := []struct {
		name     string
		args     args
		want     string
		wantErr  bool
		valueErr string
	}{
		{name: "Delete Known valid IP", args: args{clientIP: IPInCache}, want: "", wantErr: true, valueErr: CacheMiss},
		{name: "Delete Unknown valid IP", args: args{clientIP: IPNotInCache}, want: "", wantErr: true, valueErr: CacheMiss},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.Delete(tt.args.clientIP)
			got, err := client.Get(tt.args.clientIP)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Delete() = %v, want %v", got, tt.want)
				return
			}
			if tt.valueErr != "" && tt.valueErr != err.Error() {
				t.Errorf("Delete() err = %v, want %v", err.Error(), tt.valueErr)
			}
		})
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestRedisAuthenticationFailureReturns
// FR: Vérifie qu'un échec AUTH est propagé par GET, SET et DEL sans blocage
//
//	sur un canal nil ou sans lecteur.
//
// EN: Verifies that an AUTH failure is returned by GET, SET, and DEL without
//
//	blocking on a nil or unread channel.
//
// Called by: go test
// Calls: simpleredis.SimpleRedis.Get, Set, Del
// /////////////////////////////////////////////////////////////////////////////
func TestRedisAuthenticationFailureReturns(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		for range 3 {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			buffer := make([]byte, 512)
			_, _ = connection.Read(buffer)
			_, _ = connection.Write([]byte("-WRONGPASS invalid credentials\r\n"))
			_ = connection.Close()
		}
	}()

	var redisClient simpleredis.SimpleRedis
	redisClient.Init(listener.Addr().String(), "incorrect", "")
	operations := []struct {
		name string
		run  func() error
	}{
		{name: "get", run: func() error {
			_, getErr := redisClient.Get("key")
			return getErr
		}},
		{name: "set", run: func() error {
			return redisClient.Set("key", []byte("value"), 10)
		}},
		{name: "delete", run: func() error {
			return redisClient.Del("key")
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			result := make(chan error, 1)
			go func() {
				result <- operation.run()
			}()
			select {
			case operationErr := <-result:
				if operationErr == nil || operationErr.Error() != simpleredis.RedisNoAuth {
					t.Fatalf("error = %v, want %s", operationErr, simpleredis.RedisNoAuth)
				}
			case <-time.After(time.Second):
				t.Fatal("Redis operation blocked after authentication failure")
			}
		})
	}
}
