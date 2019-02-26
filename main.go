package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/keylockerbv/secrethub-go/pkg/api"
	"github.com/keylockerbv/secrethub-go/pkg/errio"
	"github.com/keylockerbv/secrethub-go/pkg/secrethub"
)

var (
	credential           string
	credentialPassphrase string
	port                 int
	client               secrethub.Client
)

func init() {
	flag.StringVar(&credential, "C", "", "(Required) SecretHub credential")
	flag.StringVar(&credentialPassphrase, "P", "", "Passphrase to unlock SecretHub credential")
	flag.IntVar(&port, "p", 8080, "HTTP port to listen on")
	flag.Parse()

	if credential == "" {
		flag.Usage()
		exit(fmt.Errorf("credential is required"))
	}

	cred, err := secrethub.NewCredential(credential, credentialPassphrase)
	if err != nil {
		exit(err)
	}

	client = secrethub.NewClient(cred, nil)
}

func main() {
	err := startHTTPServer()
	if err != nil {
		exit(err)
	}
}

func startHTTPServer() error {
	mux := mux.NewRouter()
	v1 := mux.PathPrefix("/v1/").Subrouter()

	v1.PathPrefix("/secrets/").Handler(
		http.StripPrefix("/v1/secrets/", http.HandlerFunc(handleSecret)),
	)

	fmt.Println("SecretHub Clientd started, press ^C to exit")
	return http.ListenAndServe(fmt.Sprintf(":%v", port), mux)
}

func handleSecret(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	err := api.ValidateSecretPath(path)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, err.Error())
		return
	}

	switch r.Method {
	case "GET":
		secret, err := client.Secrets().Versions().GetWithData(path)
		if err != nil {
			var errCode int

			if err, ok := err.(errio.PublicStatusError); ok {
				errCode = err.StatusCode
			}

			if errCode == 0 {
				errCode = http.StatusInternalServerError
			}

			w.WriteHeader(errCode)
			io.WriteString(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(secret.Data)
	case "POST":
		secret, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, err.Error())
			return
		}

		_, err = client.Secrets().Write(path, secret)
		if err != nil {
			var errCode int

			if err, ok := err.(errio.PublicStatusError); ok {
				errCode = err.StatusCode
			}

			switch err {
			case secrethub.ErrCannotWriteToVersion,
				secrethub.ErrEmptySecret,
				secrethub.ErrSecretTooBig:
				errCode = http.StatusBadRequest
			}

			if errCode == 0 {
				errCode = http.StatusInternalServerError
			}

			w.WriteHeader(errCode)
			io.WriteString(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusCreated)
	default:
		w.Header().Add("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func exit(err error) {
	fmt.Printf("secrethub-clientd: error: %v\n", err)
	os.Exit(1)
}
