package main

type DecoratorSyncHookRequest struct {
	Controller  map[string]interface{} `json:"controller"`
	Object      map[string]interface{} `json:"object"`
	Attachments map[string]interface{} `json:"attachments"`
}

type DecoratorSyncHookResponse struct {
	Labels             interface{}   `json:"labels"`
	Annotations        interface{}   `json:"annotations"`
	Status             interface{}   `json:"status"`
	Attachments        []interface{} `json:"attachments"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
}

type DecoratorFinalizeHookResponse struct {
	Labels             interface{}   `json:"labels"`
	Annotations        interface{}   `json:"annotations"`
	Status             interface{}   `json:"status"`
	Attachments        []interface{} `json:"attachments"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
	Finalized          bool          `json:"finalized"`
}

type CompositeSyncHookRequest struct {
	Controller map[string]interface{} `json:"controller"`
	Parent     map[string]interface{} `json:"parent"`
	Children   map[string]interface{} `json:"children"`
	Related    map[string]interface{} `json:"related"`
	Finalizing bool                   `json:"finalizing"`
}

type CompositeSyncHookResponse struct {
	Status             interface{}   `json:"status"`
	Children           []interface{} `json:"object"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
}

type CompositeFinalizeHookResponse struct {
	Status             interface{}   `json:"status"`
	Children           []interface{} `json:"object"`
	ResyncAfterSeconds float32       `json:"resyncAfterSeconds"`
	Finalized          bool          `json:"finalized"`
}

type Vni struct {
	ApiVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   map[string]string `json:"metadata"`
	Spec       map[string]int    `json:"spec"`
}
