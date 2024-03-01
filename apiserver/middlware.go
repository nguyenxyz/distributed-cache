package apiserver

import (
	"context"
	"net/http"

	"github.com/go-chi/chi"
)

func KeyExtractorMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		ctx := context.WithValue(r.Context(), "key", key)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}
