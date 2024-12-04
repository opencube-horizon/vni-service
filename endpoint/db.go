package main

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"slices"
	"time"
)

func Init(db *sql.DB) error {
	statementStr := `
	CREATE TABLE IF NOT EXISTS 
    vni_allocs (
		uid string not null,
		namespace string not null, 
        vni integer not null,
        unique (uid, namespace, vni), 
        primary key (uid, namespace)
    );
	create index if not exists vni_allocs_idx on vni_allocs(vni);
	create index if not exists vni_allocs_idx2 on vni_allocs(uid, namespace, vni);`
	statement, err := db.Prepare(statementStr)
	if err != nil {
		return err
	}
	_, err = statement.Exec()
	if err != nil {
		return err
	}

	statementStr = `
	CREATE TABLE if not exists
	vni_allocs_log (
	   uid string not null,
	   namespace string not null, 
       vni integer not null,
	   operation text not null,
	   ts datetime not null
	);`
	statement, err = db.Prepare(statementStr)
	if err != nil {
		return err
	}
	_, err = statement.Exec()
	if err != nil {
		return err
	}

	return nil
}

func getVni(db *sql.DB, uid string, namespace string) (int, error) {
	statementStr := `
	select vni
	from vni_allocs
	where uid = ? and namespace = ?;`
	statement, err := db.Prepare(statementStr)
	if err != nil {
		return -1, err
	}
	result, err := statement.Query(uid, namespace)
	if err != nil {
		return -1, err
	}
	defer result.Close()
	vni := -1

	// result might contain multiple entries; we assume they are all equivalent and only get the first
	if result.Next() {
		err := result.Scan(&vni)
		if err != nil {
			return -1, err
		}
	}
	if vni != -1 {
		return vni, nil
	}
	return -1, nil
}

func Acquire(db *sql.DB, uid string, namespace string,
	vniMin int, vniMax int,
	doLog bool) (int, error) {
	vni, err := getVni(db, uid, namespace)
	if err != nil {
		return -1, err
	}
	if vni != -1 {
		return vni, nil
	}

	// acquire lock to prevent others from interfering & causing race conditions
	lock.Lock()
	defer lock.Unlock()
	// Check for VNI again - it could have happened that another thread has created a VNI for
	//  given parameters in the meantime!
	vni, err = getVni(db, uid, namespace)
	if err != nil {
		return -1, err
	}
	if vni != -1 {
		return vni, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return -1, err
	}
	// Generate new VNI
	statement, err := tx.Prepare("select vni from vni_allocs order by vni")
	if err != nil {
		return -1, err
	}
	result, err := statement.Query()
	defer result.Close()
	if err != nil {
		return -1, err
	}
	var vnis []int
	for result.Next() {
		err := result.Scan(&vni)
		if err != nil {
			return -1, err
		}
		vnis = append(vnis, vni)
	}

	newVni := -1
	if len(vnis) == 0 {
		newVni = vniMin
	} else {
		vniTable := slices.Repeat([]bool{false}, vniMax-vniMin)
		for _, v := range vnis {
			vniTable[v-vniMin] = true
		}
		for i, b := range vniTable {
			if !b {
				newVni = vniMin + i
				break
			}
		}
	}
	if newVni == -1 {
		return -1, errors.New("no free VNI available")
	}

	// Insert new VNI
	statement, err = tx.Prepare(`insert into vni_allocs(uid, namespace, vni) 
									   values (?,?,?) returning vni;`)
	if err != nil {
		return -1, err
	}
	result, err = statement.Query(uid, namespace, newVni)
	defer result.Close()
	if err != nil {
		return -1, err
	}
	if result.Next() {
		err := result.Scan(&vni)
		if err != nil {
			return -1, err
		}
		vnis = append(vnis, vni)
	}
	if vni != newVni {
		return -1, errors.New("VNI insert failed")
	}
	if !(newVni >= vniMin && newVni < vniMax) {
		return -1, errors.New("VNI outside range")
	}

	if doLog {
		statement, err = tx.Prepare(`insert into vni_allocs_log(uid, namespace, vni, operation, ts) 
									   values (?,?,?, "insert", ?);`)
		if err != nil {
			return -1, err
		}
		_, err = statement.Exec(uid, namespace, newVni, time.Now())

		if err != nil {
			return -1, err
		}
	}

	return newVni, tx.Commit()
}

func Release(db *sql.DB, uid string, namespace string,
	doLog bool) error {
	lock.Lock()
	defer lock.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	statement, err := tx.Prepare(`delete from vni_allocs
       								    where uid = ? and namespace = ?
       								    returning vni`)
	if err != nil {
		return err
	}
	result, err := statement.Query(uid, namespace)
	defer result.Close()
	if err != nil {
		return err
	}

	vnis := make([]int, 0)
	var vni int
	if result.Next() {
		err := result.Scan(&vni)
		if err != nil {
			return err
		}
		vnis = append(vnis, vni)
	}

	if doLog {
		for _, vni := range vnis {
			statement, err = tx.Prepare(`insert into vni_allocs_log(uid, namespace, vni, operation, ts) 
									   values (?,?,?, "delete", ?);`)
			if err != nil {
				return err
			}
			_, err = statement.Exec(uid, namespace, vni, time.Now())
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
