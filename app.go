package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

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

	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/healthz", healthzHandler(host, user, password, dbname, port))
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/customers", customerHandler(host, user, password, dbname, port))

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