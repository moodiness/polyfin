package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// staleCollation is a database whose collation version, recorded when it
// was made or last refreshed, differs from the one the cluster's ICU now
// provides: after an update that brings another ICU.
type staleCollation struct {
	database, recorded, actual string
}

// refreshCollations rebuilds the indexes of every database whose
// collation version changed, then records the new version: text indexes
// built under another ICU may not follow its order, and each connection
// would warn of the mismatch. It must run before the server's database is
// created, which PostgreSQL refuses from a template1 whose version
// changed. When every version matches, the usual case, it costs one
// query.
func (c *Cluster) refreshCollations(ctx context.Context) error {
	stale, err := c.staleCollations(ctx)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	if c.Rebuilding != nil {
		c.Rebuilding(true)
		defer c.Rebuilding(false)
	}
	for _, s := range stale {
		if err := c.refreshCollation(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// staleCollations lists the databases that accept connections whose
// recorded collation version differs from the actual one.
func (c *Cluster) staleCollations(ctx context.Context) ([]staleCollation, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	conn, err := c.connect(ctx, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.Background())
	rows, _ := conn.Query(ctx, `SELECT datname, coalesce(datcollversion, ''), coalesce(actual, '')
		FROM pg_database, pg_database_collation_actual_version(oid) actual
		WHERE datallowconn AND datcollversion IS DISTINCT FROM actual
		ORDER BY datname`)
	stale, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (staleCollation, error) {
		var s staleCollation
		return s, row.Scan(&s.database, &s.recorded, &s.actual)
	})
	if err != nil {
		return nil, fmt.Errorf("read the databases' collation versions: %w", err)
	}
	return stale, nil
}

// refreshCollation rebuilds the indexes of one database, which REINDEX
// DATABASE does from within it and outside a transaction, then records
// its new collation version.
func (c *Cluster) refreshCollation(ctx context.Context, s staleCollation) error {
	c.logger().Info("The database's collation version changed: rebuilding its indexes",
		"database", s.database, "from", s.recorded, "to", s.actual)
	began := time.Now()
	conn, err := c.connect(ctx, s.database)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	name := pgx.Identifier{s.database}.Sanitize()
	if _, err := conn.Exec(ctx, "REINDEX DATABASE "+name); err != nil {
		return fmt.Errorf("rebuild the indexes of database %s after its collation version changed from %s to %s: %w",
			s.database, s.recorded, s.actual, err)
	}
	if _, err := conn.Exec(ctx, "ALTER DATABASE "+name+" REFRESH COLLATION VERSION"); err != nil {
		return fmt.Errorf("refresh the collation version of database %s: %w", s.database, err)
	}
	c.logger().Info("The database's indexes were rebuilt and its collation version refreshed",
		"database", s.database, "from", s.recorded, "to", s.actual, "took", time.Since(began).Round(time.Millisecond))
	return nil
}

// connect connects to one of the cluster's databases.
func (c *Cluster) connect(ctx context.Context, database string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, c.url(database))
	if err != nil {
		return nil, fmt.Errorf("connect to the database %s: %w", database, err)
	}
	return conn, nil
}
