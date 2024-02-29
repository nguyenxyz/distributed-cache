package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ph-ngn/nanobox/nbox"
	"github.com/ph-ngn/nanobox/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
)

type ClusterController struct {
	client nbox.NanoboxClient
}

func NewClusterController(ctx context.Context, client nbox.NanoboxClient) *ClusterController {
	return &ClusterController{client: client}
}

func (c *ClusterController) Get(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	res, err := c.client.Get(r.Context(), &nbox.GetOrPeakRequest{Key: key})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	var ew *EntryWrapper
	if e := res.GetEntry(); e != nil {
		var value interface{}
		if err := json.Unmarshal(e.Value, &value); err != nil {
			WriteJSONErrorResponse(w, r, NewInternalError(err))
			telemetry.Log().Errorf("[ClusterController/GET]: %v", err)
			return
		}

		ew = &EntryWrapper{
			Entry: e,
			Value: value,
		}
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "GET",
		Value:   ew,
		Flag:    res.GetFlag(),
	})
}

func (c *ClusterController) Peek(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	res, err := c.client.Peek(r.Context(), &nbox.GetOrPeakRequest{Key: key})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	var ew *EntryWrapper
	if e := res.GetEntry(); e != nil {
		var value interface{}
		if err := json.Unmarshal(e.Value, &value); err != nil {
			WriteJSONErrorResponse(w, r, NewInternalError(err))
			telemetry.Log().Errorf("[ClusterController/PEEK]: %v", err)
			return
		}

		ew = &EntryWrapper{
			Entry: e,
			Value: value,
		}
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "PEEK",
		Value:   ew,
		Flag:    res.GetFlag(),
	})
}

func (c *ClusterController) Set(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	var request HTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONErrorResponse(w, r, NewBadRequestError(err))
		return
	}

	b, err := json.Marshal(request.Value)
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		telemetry.Log().Errorf("[ClusterController/SET]: %v", err)
		return
	}
	var ttl time.Duration
	if len(request.TTL) > 0 {
		ttl, err = time.ParseDuration(request.TTL)
		if err != nil {
			WriteJSONErrorResponse(w, r, NewBadRequestError(err))
			return
		}
	}

	client, release, err := c.DialMaster(r.Context())
	defer release()

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	res, err := client.Set(r.Context(), &nbox.SetOrUpdateRequest{
		Key:   key,
		Value: b,
		Ttl:   durationpb.New(ttl),
	})

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponseWithStatus(w, r, http.StatusCreated, HTTPResponse{
		Command: "SET",
		Flag:    res.GetFlag(),
	})

}

func (c *ClusterController) Update(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	var request HTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONErrorResponse(w, r, NewBadRequestError(err))
		return
	}

	b, err := json.Marshal(request.Value)
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		telemetry.Log().Errorf("[ClusterController/UPDATE]: %v", err)
		return
	}

	client, release, err := c.DialMaster(r.Context())
	defer release()

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	res, err := client.Update(r.Context(), &nbox.SetOrUpdateRequest{
		Key:   key,
		Value: b,
	})

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponseWithStatus(w, r, http.StatusCreated, HTTPResponse{
		Command: "Update",
		Flag:    res.GetFlag(),
	})

}

func (c *ClusterController) DialMaster(ctx context.Context) (client nbox.NanoboxClient, release func() error, err error) {
	release = func() error { return nil }
	master, err := c.client.DiscoverMaster(ctx, &nbox.DiscoverMasterRequest{})
	if err != nil {
		return
	}

	conn, err := grpc.DialContext(ctx, master.GetFQDN(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return
	}

	return nbox.NewNanoboxClient(conn), conn.Close, nil
}

type EntryWrapper struct {
	*nbox.Entry
	Value interface{} `json:"value"`
}

type HTTPResponse struct {
	Command string      `json:"command"`
	Value   interface{} `json:"value,omitempty"`
	Flag    bool        `json:"flag,omitempty"`
}

type HTTPRequest struct {
	Value interface{} `json:"value"`
	TTL   string      `json:"ttl"`
}
