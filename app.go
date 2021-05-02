package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"text/template"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/distribution/distribution/reference"
	"github.com/distribution/distribution/registry/client"
	"github.com/distribution/distribution/registry/client/auth"
	"github.com/distribution/distribution/registry/client/transport"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/registry"
	"github.com/pkg/errors"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

func healthzHandler(host, user, password, dbname string, port int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
		db, err := sql.Open("postgres", psqlconn)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			json.NewEncoder(w).Encode(map[string]bool{"ok": false})
			return
		}

		defer db.Close()
		err = db.Ping()
		if err != nil {
			log.Printf("error communicating with database: %s \n", err)
			json.NewEncoder(w).Encode(map[string]bool{"ok": false})
			return
		}
		log.Println("Connected to db.")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func templatesHandler(host, user, password, dbname string, port int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
		db, err := sql.Open("postgres", psqlconn)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer db.Close()
		rows, err := db.Query(`SELECT "id", "template" FROM "templates"`)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var templates []map[string]string
		for rows.Next() {
			var id int
			var template string

			err = rows.Scan(&id, &template)
			if err != nil {
				log.Printf("error reading rows: %s \n", err)
				http.Error(w, err.Error(), 500)
				return
			}
			templates = append(templates, map[string]string{
				"id":       strconv.Itoa(id),
				"template": template,
			})
		}
		json.NewEncoder(w).Encode(templates)
	}
}

func customerHandler(host, user, password, dbname string, port int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
		db, err := sql.Open("postgres", psqlconn)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer db.Close()
		rows, err := db.Query(`SELECT "id", "lastname", "firstname" FROM "customers"`)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var customers []map[string]string
		for rows.Next() {
			var id int
			var lastName string
			var firstName string

			err = rows.Scan(&id, &lastName, &firstName)
			if err != nil {
				log.Printf("error reading rows: %s \n", err)
				http.Error(w, err.Error(), 500)
				return
			}
			customers = append(customers, map[string]string{
				"id":         strconv.Itoa(id),
				"last_name":  lastName,
				"first_name": firstName,
			})
		}
		json.NewEncoder(w).Encode(customers)
	}
}

func namespaceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("error getting in-cluster config: %s \n", err)
		http.Error(w, err.Error(), 500)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("error creating clientset: %s \n", err)
		http.Error(w, err.Error(), 500)
		return
	}

	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Printf("error getting namespaces: %s \n", err)
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(namespaces)
}
func namespaceFromTemplateHandler(host, user, password, dbname string, port int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// get the template from the database
		psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
		db, err := sql.Open("postgres", psqlconn)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer db.Close()
		row := db.QueryRow(`SELECT "template" FROM "templates" WHERE id = $1`, 1)
		if err != nil {
			log.Printf("error connecting to database: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		var templ string

		err = row.Scan(&templ)
		if err != nil {
			log.Printf("error reading row: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		log.Printf("template: %s \n", templ)

		var vars struct {
			Vars map[string]string `json:"Vars"`
		}

		err = json.NewDecoder(r.Body).Decode(&vars)
		if err != nil {
			log.Printf("error decoding body %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		t, err := template.New("test").Parse(templ)
		if err != nil {
			log.Printf("error parsing template %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		var buf bytes.Buffer

		err = t.Execute(&buf, vars)
		if err != nil {
			log.Printf("error executing template %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		ns := buf.String()
		log.Printf("namespace: %s \n", ns)

		var namespaceSpec v1.NamespaceApplyConfiguration
		err = yaml.Unmarshal([]byte(ns), &namespaceSpec)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("%v", namespaceSpec)

		config, err := rest.InClusterConfig()
		if err != nil {
			log.Printf("error getting in-cluster config: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Printf("error creating clientset: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		namespaces, err := clientset.CoreV1().Namespaces().Apply(context.TODO(), &namespaceSpec, metav1.ApplyOptions{FieldManager: "makedevops"})
		if err != nil {
			log.Printf("error getting namespaces: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		json.NewEncoder(w).Encode(namespaces)
	}
}

type existingTokenHandler struct {
	token string
}

func (th *existingTokenHandler) AuthorizeRequest(req *http.Request, params map[string]string) error {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", th.token))
	return nil
}

func (th *existingTokenHandler) Scheme() string {
	return "bearer"
}

func getHTTPTransport(authConfig types.AuthConfig, endpoint *url.URL, userAgent string) (http.RoundTripper, error) {
	// get the http transport, this will be used in a client to upload manifest
	base := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}

	modifiers := []transport.RequestModifier{}
	modifiers = append(modifiers, transport.NewHeaderRequestModifier(http.Header{
		"User-Agent": []string{userAgent},
	}))
	authTransport := transport.NewTransport(base)
	challengeManager, confirmedV2, err := registry.PingV2Registry(endpoint, authTransport)
	if err != nil {
		return nil, errors.Wrap(err, "error pinging v2 registry")
	}
	if !confirmedV2 {
		return nil, fmt.Errorf("unsupported registry version")
	}
	if authConfig.RegistryToken != "" {
		passThruTokenHandler := &existingTokenHandler{token: authConfig.RegistryToken}
		modifiers = append(modifiers, auth.NewAuthorizer(challengeManager, passThruTokenHandler))
	} else {
		creds := registry.NewStaticCredentialStore(&authConfig)
		// tokenHandler := auth.NewTokenHandler(authTransport, creds, "", "push", "pull")
		basicHandler := auth.NewBasicHandler(creds)
		modifiers = append(modifiers, auth.NewAuthorizer(challengeManager, basicHandler))
	}
	return transport.NewTransport(base, modifiers...), nil
}

func imagesHandler(registryUrl, username, password string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		u, err := url.Parse(registryUrl)
		if err != nil {
			log.Printf("%s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		httpTransport, err := getHTTPTransport(
			types.AuthConfig{
				Username: username,
				Password: password,
			},
			u,
			"makedevops")

		if err != nil {
			log.Printf("%s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}

		reg, err := client.NewRegistry(registryUrl, httpTransport)
		if err != nil {
			log.Printf("error connecting to registry: %s \n", err)
			http.Error(w, err.Error(), 500)
			return
		}
		ctx := context.Background()

		entries := make(map[string][]string)

		for err = nil; err != io.EOF; {
			_entries := make([]string, 10)
			_, err = reg.Repositories(ctx, _entries, "")
			if err != io.EOF {
				log.Printf("Error getting repositories: %s \n", err)
				http.Error(w, err.Error(), 500)
				return
			}
			for _, i := range _entries {
				if i != "" {
					named, err := reference.WithName(i)
					if err != nil {
						log.Printf("Error parsing repository name: %s \n", err)
						http.Error(w, err.Error(), 500)
						return
					}
					repo, err := client.NewRepository(named, registryUrl, httpTransport)
					if err != nil {
						log.Printf("Error creating repository: %s \n", err)
						http.Error(w, err.Error(), 500)
						return
					}
					tags, err := repo.Tags(ctx).All(ctx)
					if err != nil {
						log.Printf("Error getting tags: %s \n", err)
						http.Error(w, err.Error(), 500)
						return
					}
					entries[i] = tags
				}
			}
		}

		json.NewEncoder(w).Encode(entries)
	}
}

func main() {

	viper.SetConfigName("config")            // name of config file (without extension)
	viper.SetConfigType("yaml")              // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("/etc/makedevops/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.makedevops") // call multiple times to add many search paths
	viper.AddConfigPath(".")                 // optionally look for config in the working directory
	err := viper.ReadInConfig()              // Find and read the config file
	if err != nil {                          // Handle errors reading the config file
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			log.Fatalf("config file not found: %s \n", err)
		} else {
			// Config file was found but another error was produced
			log.Fatalf("Fatal error config file: %s \n", err)
		}
	}

	host := viper.GetString("pg_host")
	port := viper.GetInt("pg_port")
	user := viper.GetString("pg_user")
	password := viper.GetString("pg_password")
	dbname := viper.GetString("pg_dbname")
	resistryUrl := viper.GetString("registry_url")
	registryUser := viper.GetString("registry_user")
	registryPassword := viper.GetString("registry_password")

	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/healthz", healthzHandler(host, user, password, dbname, port))
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/customers", customerHandler(host, user, password, dbname, port))
	api.HandleFunc("/templates", templatesHandler(host, user, password, dbname, port))
	api.HandleFunc("/namespaces", namespaceHandler).Methods("GET")
	api.HandleFunc("/namespaces", namespaceFromTemplateHandler(host, user, password, dbname, port)).Methods("POST")
	api.HandleFunc("/images", imagesHandler(resistryUrl, registryUser, registryPassword))

	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type"})
	originsOk := handlers.AllowedOrigins([]string{os.Getenv("ORIGIN_ALLOWED")})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})

	srv := &http.Server{
		Addr: "0.0.0.0:8080",
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      handlers.CORS(originsOk, headersOk, methodsOk)(r), // Pass our instance of gorilla/mux in.
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)
}
