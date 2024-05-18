package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi"
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
	res, err := c.client.Get(r.Context(), &nbox.GetOrPeakRequest{Key: extractKeyFromRequest(r)})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	var dent *DecodedEntry
	if ent := res.GetEntry(); ent != nil {
		var value interface{}
		if err := json.Unmarshal(ent.GetValue(), &value); err != nil {
			WriteJSONErrorResponse(w, r, NewInternalError(err))
			telemetry.Log().Errorf("[ClusterController/GET]: %v", err)
			return
		}

		dent = &DecodedEntry{
			Entry: ent,
			Value: value,
		}
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "GET",
		Value:   dent,
		Flag:    res.GetFlag(),
	})
}

func (c *ClusterController) Peek(w http.ResponseWriter, r *http.Request) {
	res, err := c.client.Peek(r.Context(), &nbox.GetOrPeakRequest{Key: extractKeyFromRequest(r)})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	var dent *DecodedEntry
	if ent := res.GetEntry(); ent != nil {
		var value interface{}
		if err := json.Unmarshal(ent.Value, &value); err != nil {
			WriteJSONErrorResponse(w, r, NewInternalError(err))
			telemetry.Log().Errorf("[ClusterController/PEEK]: %v", err)
			return
		}

		dent = &DecodedEntry{
			Entry: ent,
			Value: value,
		}
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "PEEK",
		Value:   dent,
		Flag:    res.GetFlag(),
	})
}

func (c *ClusterController) Set(w http.ResponseWriter, r *http.Request) {
	var request HTTPRequest
	sanitize := func() error {
		var err error
		err = errors.Join(err, json.NewDecoder(r.Body).Decode(&request))
		if request.Value == nil {
			err = errors.Join(err, errors.New("value must be provided"))
		}
		if _, terr := time.ParseDuration(request.TTL); err != nil {
			err = errors.Join(err, terr)
		}

		return err
	}

	if err := sanitize(); err != nil {
		WriteJSONErrorResponse(w, r, NewBadRequestError(err))
		return
	}

	b, _ := json.Marshal(request.Value)
	ttl, _ := time.ParseDuration(request.TTL)
	client, release, err := c.DialMaster(r.Context())
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	defer release()
	res, err := client.Set(r.Context(), &nbox.SetOrUpdateRequest{
		Key:   extractKeyFromRequest(r),
		Value: b,
		Ttl:   durationpb.New(ttl),
	})

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	WriteJSONResponseWithStatus(w, r, http.StatusCreated, HTTPResponse{
		Command: "SET",
		Flag:    res.GetFlag(),
	})

}

func (c *ClusterController) Update(w http.ResponseWriter, r *http.Request) {
	var request HTTPRequest
	sanitize := func() error {
		var err error
		err = errors.Join(err, json.NewDecoder(r.Body).Decode(&request))
		if request.Value == nil {
			err = errors.Join(err, errors.New("value must be provided"))
		}

		return err
	}

	if err := sanitize(); err != nil {
		WriteJSONErrorResponse(w, r, NewBadRequestError(err))
		return
	}

	b, _ := json.Marshal(request.Value)
	client, release, err := c.DialMaster(r.Context())
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	defer release()
	res, err := client.Update(r.Context(), &nbox.SetOrUpdateRequest{
		Key:   extractKeyFromRequest(r),
		Value: b,
	})

	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	WriteJSONResponseWithStatus(w, r, http.StatusCreated, HTTPResponse{
		Command: "Update",
		Flag:    res.GetFlag(),
	})

}

func (c *ClusterController) Delete(w http.ResponseWriter, r *http.Request) {
	client, release, err := c.DialMaster(r.Context())
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	defer release()
	res, err := client.Delete(r.Context(), &nbox.DeleteOrPurgeRequest{Key: extractKeyFromRequest(r)})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "DELETE",
		Flag:    res.GetFlag(),
	})
}

func (c *ClusterController) Purge(w http.ResponseWriter, r *http.Request) {
	client, release, err := c.DialMaster(r.Context())
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	defer release()
	_, err = client.Purge(r.Context(), &nbox.DeleteOrPurgeRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "PURGE",
	})
}

func (c *ClusterController) Keys(w http.ResponseWriter, r *http.Request) {
	res, err := c.client.Keys(r.Context(), &nbox.KeysRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "KEYS",
		Value:   res.GetKeys(),
	})
}

func (c *ClusterController) Entries(w http.ResponseWriter, r *http.Request) {
	res, err := c.client.Entries(r.Context(), &nbox.EntriesRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "ENTRIES",
		Value:   res.GetEntries(),
	})
}

func (c *ClusterController) Size(w http.ResponseWriter, r *http.Request) {
	res, err := c.client.Size(r.Context(), &nbox.SizeOrCapRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "SIZE",
		Value:   res.GetSize(),
	})
}

func (c *ClusterController) Cap(w http.ResponseWriter, r *http.Request) {
	res, err := c.client.Cap(r.Context(), &nbox.SizeOrCapRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "CAP",
		Value:   res.GetSize(),
	})
}

func (c *ClusterController) Resize(w http.ResponseWriter, r *http.Request) {
	var request HTTPRequest
	sanitize := func() error {
		var err error
		err = errors.Join(err, json.NewDecoder(r.Body).Decode(&request))
		if request.Size == nil {
			err = errors.Join(errors.New("size must be provided"))
		}

		return err
	}

	if err := sanitize(); err != nil {
		WriteJSONErrorResponse(w, r, NewBadRequestError(err))
		return
	}

	client, release, err := c.DialMaster(r.Context())
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	defer release()
	_, err = client.Resize(r.Context(), &nbox.ResizeRequest{})
	if err != nil {
		WriteJSONErrorResponse(w, r, NewInternalError(err))
		return
	}

	WriteJSONResponse(w, r, HTTPResponse{
		Command: "RESIZE",
	})
}

func (c *ClusterController) DialMaster(ctx context.Context) (client nbox.NanoboxClient, release func() error, err error) {
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

type DecodedEntry struct {
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
	Size  *int        `json:"size"`
}

func extractKeyFromRequest(r *http.Request) string {
	return chi.URLParam(r, "key")
}
