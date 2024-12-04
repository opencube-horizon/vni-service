package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/tidwall/gjson"
)

var DB *sql.DB
var lock sync.Mutex
var vniMin = 100
var vniMax = 5000
var shouldLog bool

func version(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("0.1"))
}

type SyncHookRequest struct {
	Controller  map[string]interface{} `json:"controller"`
	Object      map[string]interface{} `json:"object"`
	Attachments map[string]interface{} `json:"attachments"`
}

type SyncHookResponse struct {
	Labels             interface{}   `json:"labels"`
	Annotations        interface{}   `json:"annotations"`
	Status             interface{}   `json:"status"`
	Attachments        []interface{} `json:"attachments"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
}

type FinalizeHookResponse struct {
	Labels             interface{}   `json:"labels"`
	Annotations        interface{}   `json:"annotations"`
	Status             interface{}   `json:"status"`
	Attachments        []interface{} `json:"attachments"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
	Finalized          bool          `json:"finalized"`
}

type Vni struct {
	ApiVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   map[string]string `json:"metadata"`
	Spec       map[string]int    `json:"spec"`
}

func syncEP(w http.ResponseWriter, r *http.Request) {
	log.Printf("/sync")

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var syncHookRequest SyncHookRequest
	err := json.Unmarshal(body, &syncHookRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error reading body: %v\n", err.Error())
		return
	}

	attachments := syncHookRequest.Attachments
	for k, _ := range attachments {
		fmt.Printf("Attachment: %s\n", k)
	}
	//objKind := gjson.GetBytes(body, "object.kind")
	uid := gjson.GetBytes(body, "object.metadata.uid")
	namespace := gjson.GetBytes(body, "object.metadata.namespace").String()
	//annotations := gjson.GetBytes(body, "object.metadata.annotations")

	//log.Printf("kind: %s, uid: %s, annots: %s\n", objKind.String(), uid.String(), annotations.String())

	syncHookResponse := SyncHookResponse{}
	for k, _ := range attachments {

		//fmt.Printf("key:%s,value:%s\n", k, v)
		if k == "Vni.horizon-opencube.eu/v1" {
			vniUid := fmt.Sprintf("vni-%s", uid.String())
			vni, err := Acquire(DB, vniUid, namespace, vniMin, vniMax, shouldLog)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				log.Printf("Error acquiring VNI: %v\n", err)
				return
			}
			newVni := Vni{
				ApiVersion: "horizon-opencube.eu/v1",
				Kind:       "Vni",
				Metadata:   map[string]string{"name": vniUid, "namespace": namespace},
				Spec:       map[string]int{"vni": vni},
			}
			syncHookResponse.Attachments = append(syncHookResponse.Attachments, newVni)
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(syncHookResponse)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error writing body: %v\n", err.Error())
		return
	}
	return
}

func finalizeEP(w http.ResponseWriter, r *http.Request) {
	log.Printf("/finalize")

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var syncHookRequest SyncHookRequest
	err := json.Unmarshal(body, &syncHookRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error reading body: %v\n", err.Error())
		return
	}
	//log.Printf("Body: %v\n", string(body))

	attachments := syncHookRequest.Attachments
	for k, attachment := range attachments {
		if k == "Vni.horizon-opencube.eu/v1" {
			fmt.Printf("Attachment: %s\n", k)
			vni, ok := attachment.(map[string]interface{})
			if !ok {
				log.Printf("Error handling VNI attachment: %v\n", attachment)
				continue
			}

			for uid, vniBody := range vni {
				vniBodyParsed, ok := vniBody.(map[string]interface{})
				if !ok {
					log.Printf("Error handling VNI attachment: %v\n", attachment)
					continue
				}

				metadata := vniBodyParsed["metadata"]
				metadataParsed, ok := metadata.(map[string]interface{})
				if !ok {
					log.Printf("Error handling VNI attachment: %v\n", attachment)
					continue
				}
				namespace := metadataParsed["namespace"].(string)
				err := Release(DB, uid, namespace, shouldLog)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					log.Printf("Error releasing VNI: %v\n", err)
				}
			}
		}
	}

	syncHookResponse := FinalizeHookResponse{
		Finalized:   true,
		Attachments: make([]interface{}, 0),
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(syncHookResponse)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error writing body: %v\n", err.Error())
		return
	}
}

func StartServer(filePath *string, _shouldLog bool) error {
	shouldLog = _shouldLog
	db, err := sql.Open("sqlite3", *filePath)
	defer db.Close()
	if err != nil {
		log.Printf("Error while opening sqlite file %s: %v\n", *filePath, err)
		return err
	}
	DB = db
	err = Init(DB)
	if err != nil {
		log.Fatal(err)
		return err
	}

	http.HandleFunc("/version", version)
	http.HandleFunc("/sync", syncEP)
	http.HandleFunc("/finalize", finalizeEP)

	log.Printf("Starting server at port 8842 (logging: %v)\n", shouldLog)
	err = http.ListenAndServe(":8842", nil)
	if err != nil {
		log.Printf("Error while starting server: %v\n",
			err)
		return err
	}
	return nil
}
