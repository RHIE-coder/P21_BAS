package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	

	"bserver/oauth2/errors"
	"bserver/oauth2/generates"
	"bserver/oauth2/manage"
	"bserver/oauth2/models"
	"bserver/oauth2/server"
	"bserver/oauth2/store"
	"github.com/go-session/session"
)

var (
	dumpvar   bool
	idvar     string
	secretvar string
	domainvar string
	portvar   int
)

func init() {
	flag.BoolVar(&dumpvar, "d", true, "Dump requests and responses")
	flag.StringVar(&idvar, "i", "222222", "The client id being passed in")
	flag.StringVar(&secretvar, "s", "22222222", "The client secret being passed in")
	flag.StringVar(&domainvar, "r", "http://localhost:9094", "The domain of the redirect url")
	flag.IntVar(&portvar, "p", 9096, "the base port for the server")
}

func main() {
	flag.Parse()
	if dumpvar {
		log.Println("Dumping requests")
	}
	manager := manage.NewDefaultManager()
	manager.SetAuthorizeCodeTokenCfg(manage.DefaultAuthorizeCodeTokenCfg)

	// token store
	manager.MustTokenStorage(store.NewMemoryTokenStore())

	// generate jwt access token
	// manager.MapAccessGenerate(generates.NewJWTAccessGenerate("", []byte("00000000"), jwt.SigningMethodHS512))
	manager.MapAccessGenerate(generates.NewAccessGenerate())

	clientStore := store.NewClientStore()
	clientStore.Set(idvar, &models.Client{
		ID:     idvar,
		Secret: secretvar,
		Domain: domainvar,
	})
	manager.MapClientStorage(clientStore)

	srv := server.NewServer(server.NewConfig(), manager)

	srv.SetPasswordAuthorizationHandler(func(username, password string) (userID string, err error) {
		if username == "test" && password == "test" {
			userID = "test"
		}
		return
	})

	srv.SetUserAuthorizationHandler(userAuthorizeHandler)

	srv.SetInternalErrorHandler(func(err error) (re *errors.Response) {
		log.Println("Internal Error:", err.Error())
		return
	})

	srv.SetResponseErrorHandler(func(re *errors.Response) {
		log.Println("Response Error:", re.Error.Error())
	})

	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/auth", authHandler)

	http.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("DY ------------ In : 0")
		if dumpvar {
			dumpRequest(os.Stdout, "authorize", r)
		}
		log.Printf("DY ------------ In : 1")

		store, err := session.Start(r.Context(), w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("DY ------------ In : 2")

		var form url.Values
		if v, ok := store.Get("ReturnUri"); ok {
			form = v.(url.Values)
		}
		r.Form = form
		log.Printf("DY ------------ In : 3")

		store.Delete("ReturnUri")
		store.Save()

		log.Printf("DY ------------ In : 4")

		err = srv.HandleAuthorizeRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		log.Printf("DY ------------ Out : HandleAuthorizeRequest")
	})

	http.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if dumpvar {
			_ = dumpRequest(os.Stdout, "token", r) // Ignore the error
		}

		log.Printf("DY ------------ IN : HandleTokenRequest")

		err := srv.HandleTokenRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		log.Printf("DY ------------ Out : HandleTokenRequest")
	})

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		if dumpvar {
			_ = dumpRequest(os.Stdout, "test", r) // Ignore the error
		}
		// token, err := srv.ValidationBearerToken(r)
		// if err != nil {
		// 	http.Error(w, err.Error(), http.StatusBadRequest)
		// 	return
		// }		

		// data := map[string]interface{}{
		// 	"expires_in": int64(token.GetAccessCreateAt().Add(token.GetAccessExpiresIn()).Sub(time.Now()).Seconds()),
		// 	"client_id":  token.GetClientID(),
		// 	"user_id":    token.GetUserID(),
		// 	"scope":      token.GetScope(),
		// }
		// e := json.NewEncoder(w)
		// e.SetIndent("", "  ")
		// e.Encode(data)

		err := srv.ValidationBearerToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data := map[string]interface{}{
			"photo_1": "Character:PNG:97KB:600*320",
			"photo_2": "Rabbit:PNG:75KB:600*320",
			"photo_3": "Cityscape:PNG:121KB:1200*800",
			"photo_4": "Diagram:PNG:103KB:600*320",
			"photo_5": "vehicle:PNG:93KB:600*320",
			"photo_6": "Flower:PNG:88KB:600*320",
		}
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		e.Encode(data)

		
		// >>log
		log.Printf("In /test")
	})

	log.Printf("Server is running at %d port.\n", portvar)
	log.Printf("Point your OAuth client Auth endpoint to %s:%d%s", "http://localhost", portvar, "/oauth/authorize")
	log.Printf("Point your OAuth client Token endpoint to %s:%d%s", "http://localhost", portvar, "/oauth/token")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", portvar), nil))
}

func dumpRequest(writer io.Writer, header string, r *http.Request) error {
	data, err := httputil.DumpRequest(r, true)
	if err != nil {
		return err
	}
	writer.Write([]byte("\n" + header + ": \n"))
	writer.Write(data)
	return nil
}

func userAuthorizeHandler(w http.ResponseWriter, r *http.Request) (userID string, err error) {
	if dumpvar {
		_ = dumpRequest(os.Stdout, "userAuthorizeHandler", r) // Ignore the error
	}
	store, err := session.Start(r.Context(), w, r)
	if err != nil {
		return
	}

	uid, ok := store.Get("LoggedInUserID")
	if !ok {
		if r.Form == nil {
			r.ParseForm()
		}

		store.Set("ReturnUri", r.Form)
		store.Save()

		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusFound)
		return
	}

	userID = uid.(string)
	store.Delete("LoggedInUserID")
	store.Save()
	return
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if dumpvar {
		_ = dumpRequest(os.Stdout, "login", r) // Ignore the error
	}
	store, err := session.Start(r.Context(), w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Method == "POST" {
		if r.Form == nil {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		store.Set("LoggedInUserID", r.Form.Get("username"))
		store.Save()

		w.Header().Set("Location", "/auth")
		w.WriteHeader(http.StatusFound)
		return
	}
	outputHTML(w, r, "static/login.html")
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	if dumpvar {
		_ = dumpRequest(os.Stdout, "auth", r) // Ignore the error
	}
	store, err := session.Start(nil, w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := store.Get("LoggedInUserID"); !ok {
		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusFound)
		return
	}

	outputHTML(w, r, "static/auth.html")
}

func outputHTML(w http.ResponseWriter, req *http.Request, filename string) {
	file, err := os.Open(filename)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer file.Close()
	fi, _ := file.Stat()
	http.ServeContent(w, req, file.Name(), fi.ModTime(), file)
}
