package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// passwordVariable is the variable the PostgreSQL driver, pg_dump and
// pg_restore read the database's password from when the connection string
// gives none.
const passwordVariable = "PGPASSWORD"

// errPasswordStays reports a connection string the password of PGPASSWORD
// could not be written into.
var errPasswordStays = errors.New("the driver does not read the database URL the same once the password of PGPASSWORD is written in it")

// carryPassword takes PGPASSWORD out of this process's environment, which
// the programs Polyfin runs inherit, when environ, the environment this
// process was started with, sets it, and returns the connection string
// that reads the same without it. The driver reads the variable each time
// it reads a connection string, and pg_dump inherits it, so the password
// it gives must be in the string from then on:
//   - connection itself, when the driver reads it the same without the
//     variable, as the string or a service file gives the password, and
//     when Polyfin runs its own database, whose address has its password;
//   - connection with the variable's password written in it (see
//     withPassword), once the driver has read it into the very
//     configuration it read connection into with the variable set.
//
// When it has not, the variable stays, connection is returned as it is,
// and errPasswordStays tells: Polyfin starts as it did before. A
// connection string the driver cannot read is left to the start to refuse.
func carryPassword(connection string, environ []string) (string, error) {
	return carryPasswordWith(connection, environ, withPassword)
}

// carryPasswordWith is carryPassword where write writes the password into
// the connection string.
func carryPasswordWith(connection string, environ []string, write func(connection, password string) string) (string, error) {
	password, set := os.LookupEnv(passwordVariable)
	if !set || !slices.ContainsFunc(environ, func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return strings.EqualFold(name, passwordVariable)
	}) {
		return connection, nil
	}
	if connection == "" || password == "" {
		_ = os.Unsetenv(passwordVariable)
		return connection, nil
	}
	with, err := pgconn.ParseConfig(connection)
	if err != nil {
		return connection, nil
	}
	_ = os.Unsetenv(passwordVariable)
	if without, err := pgconn.ParseConfig(connection); err == nil && sameConnection(with, without) {
		return connection, nil
	}
	carried := write(connection, password)
	if read, err := pgconn.ParseConfig(carried); err == nil && sameConnection(with, read) {
		return carried, nil
	}
	_ = os.Setenv(passwordVariable, password)
	return connection, errPasswordStays
}

// withPassword is connection, a URL or keyword=value pairs as the driver
// reads them, with password written in it: as the password of a URL's
// user, percent-encoded, or as a quoted pair ahead of the others, which
// whatever ends connection cannot change.
func withPassword(connection, password string) string {
	for _, scheme := range []string{"postgresql://", "postgres://"} {
		rest, ok := strings.CutPrefix(connection, scheme)
		if !ok {
			continue
		}
		// As the driver reads it, the user and its password end at the
		// first @ before the path, and a password starts at their first
		// colon.
		user := ""
		if i := strings.IndexAny(rest, "@/"); i >= 0 && rest[i] == '@' {
			user, _, _ = strings.Cut(rest[:i], ":")
			rest = rest[i+1:]
		}
		var encoded strings.Builder
		for _, c := range []byte(password) {
			if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
				encoded.WriteByte(c)
				continue
			}
			const hex = "0123456789ABCDEF"
			encoded.Write([]byte{'%', hex[c>>4], hex[c&0xf]})
		}
		return scheme + user + ":" + encoded.String() + "@" + rest
	}
	return "password='" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(password) + "' " + connection
}

// sameConnection reports whether the driver read two connection strings
// into the same configuration: every exported field of it, of its
// fallback hosts and of their TLS settings, where functions are compared
// by their presence and certificate pools by what they hold. The string
// each was read from, which the configuration keeps, is not compared.
func sameConnection(a, b *pgconn.Config) bool {
	return sameValue(reflect.ValueOf(a), reflect.ValueOf(b))
}

// sameValue is sameConnection for one value of a configuration.
func sameValue(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.Func:
		return a.IsNil() == b.IsNil()
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		if pool, ok := a.Interface().(*x509.CertPool); ok {
			return pool.Equal(b.Interface().(*x509.CertPool))
		}
		return sameValue(a.Elem(), b.Elem())
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		for i := range a.Len() {
			if !sameValue(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		switch a.Type() {
		case reflect.TypeFor[pgconn.Config](), reflect.TypeFor[pgconn.FallbackConfig](), reflect.TypeFor[tls.Config]():
			for i := range a.NumField() {
				if a.Type().Field(i).IsExported() && !sameValue(a.Field(i), b.Field(i)) {
					return false
				}
			}
			return true
		}
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}
