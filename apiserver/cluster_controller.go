package nboxapiserver

import (
	"context"
	"net/http"

	"github.com/ph-ngn/nanobox/nbox"
)

type ClusterController struct {
	client nbox.NanoboxClient
}

func NewClusterController(ctx context.Context, client nbox.NanoboxClient) *ClusterController {
	return &ClusterController{client: client}
}

func (c *ClusterController) Get(w http.ResponseWriter, r *http.Request) {

}
