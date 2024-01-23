package box

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ph-ngn/nanobox/pkg/util/log"
	rw "github.com/ph-ngn/nanobox/pkg/util/response"

	"github.com/go-chi/chi/v5"
)

type Controller struct {
	store Store

	l log.Logger
}

type AddRecordRequest struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type RetrieveRecordResponse struct {
	Key          string                 `json:"key"`
	Value        interface{}            `json:"value"`
	LastUpdated  time.Time              `json:"lastUpdated"`
	CreationTime time.Time              `json:"creationTime"`
	Metadata     map[string]interface{} `json:"metadata"`
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

func (c *Controller) RetrieveRecord(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if item := c.store.Get(key); item != nil {
		response := RetrieveRecordResponse{
			Key:          item.Key(),
			Value:        item.Value(),
			LastUpdated:  item.LastUpdated(),
			CreationTime: item.CreationTime(),
			Metadata:     item.Metadata(),
		}
		rw.WriteJSONResponse(w, r, 200, response)
		return
	}

}

func (c *Controller) RemoveRecord(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := c.store.Delete(key); err != nil {
		rw.WriteJSONResponse(w, r, 200, "")
		return
	}
}
