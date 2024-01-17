package box

import (
	"encoding/json"
	"log"
	"net/http"

	rw "github.com/ph-ngn/nanobox/pkg/util/responsewriter"
)

type Controller struct {
	store Store

	l log.Logger
}

type AddRecordRequest struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

func (c *Controller) AddRecord(w http.ResponseWriter, r *http.Request) {
	var request AddRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
	}

	err := c.store.Set(request.Key, request.Value)
	if err != nil {
	}

	rw.WriteJSONResponse(w, r, 200, request)

}
