package responsewriter

import (
	"encoding/json"
	"net/http"
)

func WriteJSONResponse(w http.ResponseWriter, _ *http.Request, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	return nil
}
