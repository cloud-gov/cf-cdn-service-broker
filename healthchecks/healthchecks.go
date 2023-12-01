package healthchecks

import (
	"fmt"
	"net/http"

	"github.com/alphagov/paas-cdn-broker/config"

	"github.com/jinzhu/gorm"
)

func Bind(mux *http.ServeMux, settings config.Settings, db *gorm.DB) {
	var checks = map[string]func(config.Settings) error{
		"cloudfront":   Cloudfront,
		"cloudfoundry": Cloudfoundry,
		"postgresql":   CreatePostgresqlChecker(db),
	}

	mux.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		body := ""
		for name, function := range checks {
			err := function(settings)
			if err != nil {
				body = body + fmt.Sprintf("%s error: %s\n", name, err)
			}
		}
		if body != "" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", body)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	mux.HandleFunc("/healthcheck/http", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for name, function := range checks {
		mux.HandleFunc("/healthcheck/"+name, func(w http.ResponseWriter, r *http.Request) {
			err := function(settings)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "%s error: %s", name, err)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		})
	}
}
