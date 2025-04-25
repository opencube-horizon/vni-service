package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tidwall/gjson"
	"io"
	"log"
	"net/http"
	"strings"
)

func cVersion(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("1.0"))
}

func cSync(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)

	var syncHookRequest DecoratorSyncHookRequest
	err := json.Unmarshal(body, &syncHookRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error reading body: %v\n", err.Error())
		return
	}
	attachments := syncHookRequest.Attachments

	callerKind := gjson.GetBytes(body, "object.kind").String()
	callerApiVersion := gjson.GetBytes(body, "object.apiVersion").String()
	callerUid := gjson.GetBytes(body, "object.metadata.uid").String()
	callerNamespace := gjson.GetBytes(body, "object.metadata.namespace").String()

	callerAnnotations := gjson.GetBytes(body, "object.metadata.annotations").Map()
	var callerAnnotationVni string
	if callerAnnotations != nil {
		callerAnnotationVni = strings.ToLower(callerAnnotations["vni"].String())
	}

	syncHookResponse := DecoratorSyncHookResponse{}

	db, err := open(DBFilePath)
	if err != nil {
		log.Printf("Error opening db: %v\n", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer db.Close()

	for k, _ := range attachments {
		if k == "Vni.horizon-opencube.eu/v1" {
			var vniUid string
			if callerApiVersion == "horizon-opencube.eu/v1" && callerKind == "VniClaim" {
				vniUid = gjson.GetBytes(body, "object.spec.name").String()
			} else {
				vniUid = fmt.Sprintf("vni-%s", callerUid)
			}

			if (callerApiVersion == "horizon-opencube.eu/v1" && callerKind == "VniClaim") ||
				callerAnnotationVni == "true" || callerAnnotationVni == "yes" {
				// we own the VNI - create one
				vni, err := Acquire(db, vniUid, callerNamespace, vniMin, vniMax, shouldLog)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					log.Printf("Error acquiring VNI: %v\n", err)
					return
				}
				newVni := Vni{
					ApiVersion: "horizon-opencube.eu/v1",
					Kind:       "Vni",
					Metadata:   map[string]string{"name": vniUid, "namespace": callerNamespace},
					Spec:       map[string]int{"vni": vni},
				}
				syncHookResponse.Attachments = append(syncHookResponse.Attachments, newVni)
			} else if callerAnnotationVni != "" {
				// update target VNI by adding callerUid to user table
				//  only do that for non-VniClaims

				err := AddUser(db, callerAnnotationVni, callerNamespace, callerUid, shouldLog)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					log.Printf("Error adding user: %v (%s %s %s)\n",
						err, callerAnnotationVni, callerNamespace, callerUid)
					return
				}

				vni, err := GetVni(db, callerAnnotationVni, callerNamespace)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					log.Printf("Error getting VNI: %s (%s %s)\n",
						vniUid, callerNamespace, callerUid)
					return
				}

				if vni == -1 {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(err.Error()))
					log.Printf("No VNI for VNI UID: %s (%s %s)\n",
						vniUid, callerNamespace, callerUid)
					return
				}

				virtualVniUid := fmt.Sprintf("vni-%s", callerUid)
				newVni := Vni{
					ApiVersion: "horizon-opencube.eu/v1",
					Kind:       "Vni",
					Metadata:   map[string]string{"name": virtualVniUid, "namespace": callerNamespace},
					Spec:       map[string]int{"vni": vni},
				}
				syncHookResponse.Attachments = append(syncHookResponse.Attachments, newVni)
			}
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
}

func cFinalize(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)

	var syncHookRequest DecoratorSyncHookRequest
	err := json.Unmarshal(body, &syncHookRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error reading body: %v\n", err.Error())
		return
	}

	callerKind := gjson.GetBytes(body, "object.kind").String()
	callerApiVersion := gjson.GetBytes(body, "object.apiVersion").String()

	callerAnnotations := gjson.GetBytes(body, "object.metadata.annotations").Map()
	var callerAnnotationVni string
	if callerAnnotations != nil {
		callerAnnotationVni = strings.ToLower(callerAnnotations["vni"].String())
	}

	callerUid := gjson.GetBytes(body, "object.metadata.uid").String()
	callerNamespace := gjson.GetBytes(body, "object.metadata.namespace").String()

	db, err := open(DBFilePath)
	if err != nil {
		log.Printf("Error opening db: %v\n", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer db.Close()

	finalized := true
	attachments := syncHookRequest.Attachments
	for k, attachment := range attachments {
		if k == "Vni.horizon-opencube.eu/v1" {
			vni, ok := attachment.(map[string]interface{})
			if !ok {
				log.Printf("Error handling VNI attachment: %v\n", attachment)
				continue
			}

			for vniUid, vniBody := range vni {
				// Metacontroller wants to have finalized=false in case the received state
				//  does not match the desired state, yet
				//  so if there are still attached VNIs, set finalized to false
				finalized = false

				vniUidParts := strings.Split(vniUid, "-")

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
				vniNamespace := metadataParsed["namespace"].(string)

				if callerApiVersion == "horizon-opencube.eu/v1" && callerKind == "VniClaim" {
					// we are a VniClaim - only release VNI if no other users are using it

					err = ReleaseUserCheck(db, vniUid, vniNamespace, shouldLog)
					if errors.Is(err, ErrVNINotFound) {
						continue
					} else if errors.Is(err, ErrVNIInUse) {
						w.Write([]byte("VNI still in use, will not release\n"))
						log.Printf("VNI still in use, will not release\n")
						finalized = false
						continue
					} else if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte(err.Error()))
						log.Printf("Error releasing VNI: %v\n", err)
						return
					}
				} else if (callerAnnotationVni == "true" || callerAnnotationVni == "yes") &&
					(len(vniUidParts) > 1 && strings.Join(vniUidParts[1:], "-") == callerUid) {
					// we requested a VNI & own it - release it immediately

					err = ReleaseUserCheck(db, vniUid, vniNamespace, shouldLog)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte(err.Error()))
						log.Printf("Error releasing VNI: %v\n", err)
						return
					}
				} else if callerAnnotationVni != "" &&
					!(callerAnnotationVni == "true" || callerAnnotationVni == "yes") &&
					fmt.Sprintf("vni-%s", callerUid) == vniUid {
					// we are a Job et al. and redeem a VNI Claim
					//  update target VNI by removing callerUid from user table

					err := RemoveUser(db, callerAnnotationVni, callerNamespace, callerUid, shouldLog)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte(err.Error()))
						log.Printf("Error removing user: %v\n", err)
						return
					}
				}
			}
		}
	}

	syncHookResponse := DecoratorFinalizeHookResponse{
		Finalized:   finalized,
		Attachments: make([]interface{}, 0),
	}
	if !finalized {
		syncHookResponse.ResyncAfterSeconds = 5
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(syncHookResponse)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		log.Printf("Error writing body: %v\n", err.Error())
		return
	}
}
