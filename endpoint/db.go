package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"time"
)

var ErrVNINotFound = errors.New("VNI not found")
var ErrNoFreeVNI = errors.New("no free VNI available")
var ErrVNIInUse = errors.New("VNI still in use")

func open(filePath *string) (db *sql.DB, err error) {
	db, err = sql.Open("sqlite3_with_extensions", *filePath)
	return db, err
}

func Init(db *sql.DB) error {
	ctx := context.TODO()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	// vni_allocs
	_, err = tx.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS 
    vni_allocs (
		vniUid string not null,
		namespace string not null, 
        vni integer not null,
        unique (vniUid, namespace, vni), 
        primary key (vniUid, namespace)
    );
	create index if not exists vni_allocs_idx on vni_allocs(vni);
	create index if not exists vni_allocs_idx2 on vni_allocs(vniUid, namespace, vni);`)
	if err != nil {
		return err
	}

	// vni_allocs_log
	_, err = tx.ExecContext(ctx, `
	CREATE TABLE if not exists
	vni_allocs_log (
	   vniUid string not null,
	   namespace string not null, 
       vni integer not null,
	   operation text not null,
	   ts datetime not null
	);`)
	if err != nil {
		return err
	}

	// vni_users
	_, err = tx.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS 
    vni_users (
		vniUid string not null,
		namespace string not null, 
        userId text not null,
        unique (vniUid, namespace, userId), 
        primary key (vniUid, namespace, userId)
    );
	create index if not exists vni_users_idx on vni_users(vniUid, namespace, userId);`)
	if err != nil {
		return err
	}

	// vni_users_log
	_, err = tx.ExecContext(ctx, `
	CREATE TABLE if not exists
	vni_users_log (
	   vniUid string not null,
	   namespace string not null, 
       userId text not null,
	   operation text not null,
	   ts datetime not null
	);`)
	if err != nil {
		return err
	}

	// available_vnis
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`
	CREATE TABLE if not exists
	available_vnis (
		vni int not null primary key,
		lastReleased datetime,
		unique (vni),
	    check (vni >= %d and vni < %d)
	);`, vniMin, vniMax))
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
	insert or ignore into available_vnis (vni, lastReleased)
	    select value, null as vni from generate_series(?, ?, 1)
	;`, vniMin, vniMax)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func GetVni(db *sql.DB, vniUid string, namespace string) (int, error) {
	vni := -1
	err := db.QueryRowContext(context.TODO(), `
	select vni
	from vni_allocs
	where vniUid = ? and namespace = ?;`,
		vniUid, namespace).Scan(&vni)

	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	return vni, err
}

func Acquire(db *sql.DB, vniUid string, namespace string,
	vniMin int, vniMax int,
	doLog bool) (int, error) {
	vni, err := GetVni(db, vniUid, namespace)
	if err != nil {
		return -1, err
	}
	if vni != -1 {
		return vni, nil
	}

	ctx := context.TODO()

	result, err := db.QueryContext(ctx, `
with free_vnis as (
	select vni 
		from available_vnis
		where unixepoch(datetime('now')) - coalesce(unixepoch(lastReleased), 0) > 60
	except
	select vni from vni_allocs
 ),
new_vni as (
	select min(vni) as vni 
	from free_vnis
)

insert into vni_allocs (vniUid, namespace, vni)
select 
    ?,
    ?, 
    vni 
from new_vni
where vni > 0
returning vni;
`, vniUid, namespace)
	if err != nil {
		return -1, err
	}
	defer result.Close()
	if !result.Next() {
		return -1, errors.New("no result rows in VNI creation")
	}
	var newVni int
	err = result.Scan(&newVni)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, ErrNoFreeVNI
	}
	if err != nil {
		return -1, err
	}
	result.Close()

	if !(newVni >= vniMin && newVni < vniMax) {
		return -1, errors.New("VNI outside range")
	}

	if doLog {
		_, err = db.ExecContext(ctx, `insert into vni_allocs_log(vniUid, namespace, vni, operation, ts) 
									   values (?,?,?, "acquire", ?);`,
			vniUid, namespace, newVni, time.Now())
		if err != nil {
			return -1, err
		}
	}

	return newVni, nil
}

func ReleaseUserCheck(db *sql.DB, vniUid string, namespace string,
	doLog bool) error {

	ctx := context.TODO()
	vni, err := GetVni(db, vniUid, namespace)
	if err != nil {
		return err
	}
	if vni == -1 {
		return ErrVNINotFound
	}

	_, err = db.ExecContext(ctx, `
update available_vnis
set lastReleased = datetime('now')
where vni in (
    select vni
	from vni_allocs
	where vniUid = ? and namespace = ?
);
`, vniUid, namespace)
	if err != nil {
		return err
	}

	result, err := db.QueryContext(ctx, `
delete from vni_allocs
where vniUid = ? and namespace = ?
and vniUid not in (
    select vniUid
	from vni_users
	where vniUid = ? and namespace = ?
)
returning vni;
`, vniUid, namespace, vniUid, namespace)
	if err != nil {
		return err
	}
	defer result.Close()

	vnis := make([]int, 0)
	for result.Next() {
		var _vni int
		if err := result.Scan(&_vni); err != nil {
			return err
		}
		vnis = append(vnis, _vni)
	}

	if err := result.Close(); err != nil {
		return err
	}
	if len(vnis) == 0 { // query returns deleted VNIs, so if len == 0, none were deleted
		return ErrVNIInUse
	}

	if doLog {
		for _, vni := range vnis {
			_, err = db.ExecContext(ctx, `insert into vni_allocs_log(vniUid, namespace, vni, operation, ts) 
									   values (?,?,?, "release", ?);`,
				vniUid, namespace, vni, time.Now())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AddUser(db *sql.DB, vniUid string, namespace string, userId string, doLog bool) error {
	ctx := context.TODO()

	isPresent, err := getUser(db, vniUid, namespace, userId)
	if err != nil {
		return err
	}
	if isPresent {
		return nil
	}

	result, err := db.QueryContext(ctx, `
with vni_search as (
	select vniUid, namespace
	from vni_allocs
	where vniUid = ? and namespace = ?
)

insert or ignore into vni_users (vniUid, namespace, userId)  
select vniUid, namespace, ?
from vni_search
returning vniUid;`, vniUid, namespace, userId)
	if err != nil {
		return err
	}
	defer result.Close()
	if !result.Next() {
		return ErrVNINotFound
	}
	if err := result.Close(); err != nil {
		return err
	}

	if doLog {
		_, err = db.ExecContext(ctx, `insert into vni_users_log(vniUid, namespace, userId, operation, ts) 
									   values (?,?,?, "add", ?);`,
			vniUid, namespace, userId, time.Now())
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveUser(db *sql.DB, vniUid string, namespace string, userId string, doLog bool) error {
	//lock.Lock()
	//defer lock.Unlock()

	ctx := context.TODO()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelLinearizable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		delete from vni_users
		where vniUid = ? and namespace = ? and userId = ?;`, vniUid, namespace, userId)
	if err != nil {
		return err
	}

	if doLog {
		_, err = tx.ExecContext(ctx, `insert into vni_users_log(vniUid, namespace, userId, operation, ts) 
									   values (?,?,?, "remove", ?);`,
			vniUid, namespace, userId, time.Now())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func getUser(db *sql.DB, vniUid string, namespace string, userId string) (bool, error) {
	dbEntry := ""
	err := db.QueryRowContext(context.TODO(), `
	select userId
	from vni_users
	where userId = ? and vniUid = ? and namespace = ?;`, userId, vniUid, namespace).
		Scan(&dbEntry)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	return dbEntry != "", err
}
