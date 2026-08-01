// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux hardened SimpleRedis client
// FR: Source dérivée de simpleredis v1.0.12 et modifiée afin de propager les
//
//	    erreurs de
//
//		protocole et d'authentification sans blocage, avec des commandes RESP
//		binaires et des délais réseau bornés.
//
// EN: Source derived from simpleredis v1.0.12 and modified to propagate
//
//	    protocol and
//
//		authentication failures without blocking, using binary-safe RESP commands
//		and bounded network deadlines.
//
// Called by: PareFlux Gate Redis cache
// Calls: net.Dialer, bufio.Reader, bufio.Writer
// /////////////////////////////////////////////////////////////////////////////
// Package simpleredis implements utility routines for interacting with Redis.
package simpleredis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Error strings for Redis.
const (
	RedisUnreachable = "redis:unreachable"
	RedisMiss        = "redis:miss"
	RedisTimeout     = "redis:timeout"
	RedisNoAuth      = "redis:noauth"
	RedisIssue       = "redis:issue?"
)

// A redisCmd is used to communicate with Redis at low level.
type redisCmd struct {
	Command  string
	Name     string
	Data     []byte
	Duration int64
	Error    error
}

// A SimpleRedis is used to communicate with Redis.
type SimpleRedis struct {
	host     string
	pass     string
	database string
}

// /////////////////////////////////////////////////////////////////////////////
// writeRedisCommand
// FR: Encode une commande au format RESP et vide explicitement le tampon afin
//
//	de préserver les espaces, retours à la ligne et octets arbitraires.
//
// EN: Encodes a command as RESP and explicitly flushes the buffer so spaces,
//
//	line breaks, and arbitrary bytes are preserved.
//
// Called by: SimpleRedis.askRedis
// Calls: bufio.Writer.Write, bufio.Writer.Flush
// /////////////////////////////////////////////////////////////////////////////
func writeRedisCommand(writer *bufio.Writer, arguments ...[]byte) error {
	if _, err := fmt.Fprintf(writer, "*%d\r\n", len(arguments)); err != nil {
		return err
	}
	for _, argument := range arguments {
		if _, err := fmt.Fprintf(writer, "$%d\r\n", len(argument)); err != nil {
			return err
		}
		if _, err := writer.Write(argument); err != nil {
			return err
		}
		if _, err := writer.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// /////////////////////////////////////////////////////////////////////////////
// readRedisResponse
// FR: Lit une réponse RESP simple, entière ou bulk et transforme les erreurs
//
//	d'authentification et les absences de clé en erreurs stables.
//
// EN: Reads a simple, integer, or bulk RESP response and maps authentication
//
//	failures and missing keys to stable errors.
//
// Called by: SimpleRedis.askRedis
// Calls: bufio.Reader.ReadByte, bufio.Reader.ReadString, io.ReadFull
// /////////////////////////////////////////////////////////////////////////////
func readRedisResponse(reader *bufio.Reader) ([]byte, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")

	switch prefix {
	case '+', ':':
		return []byte(line), nil
	case '-':
		if strings.Contains(line, "NOAUTH") || strings.Contains(line, "WRONGPASS") {
			return nil, errors.New(RedisNoAuth)
		}
		return nil, errors.New(RedisIssue)
	case '$':
		length, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			return nil, errors.New(RedisIssue)
		}
		if length == -1 {
			return nil, errors.New(RedisMiss)
		}
		if length < 0 {
			return nil, errors.New(RedisIssue)
		}
		data := make([]byte, length+2)
		if _, readErr := io.ReadFull(reader, data); readErr != nil {
			return nil, readErr
		}
		if data[length] != '\r' || data[length+1] != '\n' {
			return nil, errors.New(RedisIssue)
		}
		return data[:length], nil
	default:
		return nil, errors.New(RedisIssue)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// askRedis
// FR: Exécute une commande Redis sur une connexion à durée bornée et arrête
//
//	immédiatement si AUTH ou SELECT échoue.
//
// EN: Executes a Redis command over a deadline-bounded connection and stops
//
//	immediately when AUTH or SELECT fails.
//
// Called by: SimpleRedis.Get, SimpleRedis.Set, SimpleRedis.Del
// Calls: writeRedisCommand, readRedisResponse
// /////////////////////////////////////////////////////////////////////////////
func (sr *SimpleRedis) askRedis(cmd redisCmd) redisCmd {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	connection, err := dialer.Dial("tcp", sr.host)
	if err != nil {
		return redisCmd{Error: errors.New(RedisUnreachable)}
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return redisCmd{Error: errors.New(RedisTimeout)}
	}

	writer := bufio.NewWriter(connection)
	reader := bufio.NewReader(connection)
	execute := func(arguments ...[]byte) ([]byte, error) {
		if writeErr := writeRedisCommand(writer, arguments...); writeErr != nil {
			return nil, writeErr
		}
		return readRedisResponse(reader)
	}

	if sr.pass != "" {
		if _, err = execute([]byte("AUTH"), []byte(sr.pass)); err != nil {
			return redisCmd{Error: normalizeRedisError(err)}
		}
	}
	if sr.database != "" {
		if _, err = execute([]byte("SELECT"), []byte(sr.database)); err != nil {
			return redisCmd{Error: normalizeRedisError(err)}
		}
	}

	var data []byte
	switch cmd.Command {
	case "SET":
		data, err = execute(
			[]byte("SET"), []byte(cmd.Name), cmd.Data,
			[]byte("EX"), []byte(strconv.FormatInt(cmd.Duration, 10)),
		)
	case "DEL":
		data, err = execute([]byte("DEL"), []byte(cmd.Name))
	case "GET":
		data, err = execute([]byte("GET"), []byte(cmd.Name))
	default:
		err = errors.New(RedisIssue)
	}
	if err != nil {
		return redisCmd{Error: normalizeRedisError(err)}
	}
	return redisCmd{Data: data}
}

// /////////////////////////////////////////////////////////////////////////////
// normalizeRedisError
// FR: Conserve les erreurs Redis publiques et masque les détails réseau
//
//	instables derrière une erreur de délai ou de disponibilité.
//
// EN: Preserves public Redis errors and maps unstable network details to
//
//	timeout or availability errors.
//
// Called by: SimpleRedis.askRedis
// Calls: errors.Is, net.Error
// /////////////////////////////////////////////////////////////////////////////
func normalizeRedisError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []string{RedisNoAuth, RedisMiss, RedisIssue} {
		if err.Error() == known {
			return err
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New(RedisTimeout)
	}
	return errors.New(RedisUnreachable)
}

// Init sets the Redis host used to connect to Redis.
func (sr *SimpleRedis) Init(host, pass, database string) {
	sr.host = host
	sr.pass = pass
	sr.database = database
}

// Get fetches the value for key name in Redis.
func (sr *SimpleRedis) Get(name string) ([]byte, error) {
	response := sr.askRedis(redisCmd{Command: "GET", Name: name})
	return response.Data, response.Error
}

// Set updates the value for key name in Redis with value data for duration.
func (sr *SimpleRedis) Set(name string, data []byte, duration int64) error {
	response := sr.askRedis(redisCmd{
		Command:  "SET",
		Name:     name,
		Data:     data,
		Duration: duration,
	})
	return response.Error
}

// Del removes the key name in Redis.
func (sr *SimpleRedis) Del(name string) error {
	response := sr.askRedis(redisCmd{Command: "DEL", Name: name})
	return response.Error
}
