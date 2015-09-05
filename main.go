package main

import (
	"encoding/json"
	"flag"
	"fmt"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type watsonMTClient struct {
	c           *http.Client
	apiEndpoint string
	apiUser     string
	apiPwd      string
}

func (wmt *watsonMTClient) translate(source, target, text string) (string, error) {
	var translation string
	url := fmt.Sprintf("%s/v2/translate?source=%s&target=%s&text=%s", wmt.apiEndpoint, source, target, url.QueryEscape(text))
	rq, err := http.NewRequest("POST", url, strings.NewReader(text))
	if err != nil {
		return translation, err
	}
	rq.SetBasicAuth(wmt.apiUser, wmt.apiPwd)
	rq.Header.Set("content-type", "plain/text")
	resp, err := wmt.c.Do(rq)
	if err != nil {
		return translation, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return translation, err
	}
	translation = string(b)
	return translation, nil
}

// HTTP Server that implements the persistent hash table.
type server struct {
	dbSession *mgo.Session
	dbName    string
	wc        *watsonMTClient
}

func (s *server) put(v string) (bson.ObjectId, error) {
	sess := s.dbSession.Copy()
	defer sess.Close()
	c := sess.DB(s.dbName).C("dkv")
	k := bson.NewObjectId()
	err := c.Insert(&struct {
		ID   bson.ObjectId `bson:"_id"`
		Data string
	}{
		ID:   k,
		Data: v,
	})
	return k, err
}

func (s *server) get(k string) (string, error) {
	var data struct {
		ID   bson.ObjectId `bson:"_id"`
		Data string
	}
	if !bson.IsObjectIdHex(k) {
		return data.Data, mgo.ErrNotFound
	}
	sess := s.dbSession.Copy()
	defer sess.Close()
	c := sess.DB(s.dbName).C("dkv")
	err := c.Find(bson.M{"_id": bson.ObjectIdHex(k)}).One(&data)
	return data.Data, err
}

func (s *server) storageHandler(w http.ResponseWriter, r *http.Request) {
	urlcomp := strings.Split(r.URL.Path, "/")
	switch r.Method {
	case "GET":
		if len(urlcomp) < 3 {
			http.Error(w, "no data found", http.StatusNotFound)
			return
		}
		data, err := s.get(urlcomp[2])
		switch err {
		case nil:
			fmt.Fprintf(w, "{\n	k: \"%s\",\n	v: \"%s\"\n}\n", urlcomp[2], data)
		case mgo.ErrNotFound:
			http.Error(w, "no data found", http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case "POST":
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		key, err := s.put(string(data))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "{\n	Key: \"%s\"\n}\n", key.Hex())
		return
	default:
		whdr := w.Header()
		whdr.Add("Allow", "GET,POST")
		http.Error(w, "invalid http method", http.StatusMethodNotAllowed)
		return
	}
}

func (s *server) translateHandler(w http.ResponseWriter, r *http.Request) {
	urlcomp := strings.Split(r.URL.Path, "/")
	switch r.Method {
	case "GET":
		if len(urlcomp) < 4 {
			http.Error(w, "no data found", http.StatusNotFound)
			return
		}
		data, err := s.get(urlcomp[3])
		switch err {
		case nil:
		case mgo.ErrNotFound:
			http.Error(w, "no data found", http.StatusNotFound)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		translations := map[string]string{}
		for _, lang := range []string{"es", "fr", "pt"} {
			txl, err := s.wc.translate("en", lang, data)
			if err != nil {
				log.Println(err)
				continue
			}
			translations[lang] = txl
		}
		resp, err := json.MarshalIndent(&struct {
			K            string            `json:"k"`
			V            string            `json:"v"`
			Translations map[string]string `json:"translations"`
		}{
			K:            urlcomp[3],
			V:            data,
			Translations: translations,
		}, "", "	")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s\n", resp)
		return
	default:
		whdr := w.Header()
		whdr.Add("Allow", "GET")
		http.Error(w, "invalid http method", http.StatusMethodNotAllowed)
		return
	}
}

func main() {
	flagIP := flag.String("ip", "127.0.0.1", "ip address")
	flagPort := flag.String("port", "6060", "tcp port")
	flagMongo := flag.String("mongo", "localhost:27017/dkv", "mongodb URL")
	flagWatsonURL := flag.String("wurl", "https://gateway.watsonplatform.net/language-translation/api", "Watson translation service API endpoint")
	flagWatsonUser := flag.String("wuser", "de1e5046-c78a-4b4a-889e-9b016d0e2b14", "Watson translation service user name")
	flagWatsonPwd := flag.String("wpwd", "oRj6v2QWqONC", "Watson translation service password")
	flag.Parse()

	if ip := os.Getenv("VCAP_APP_HOST"); ip != "" {
		*flagIP = ip
	}
	if port := os.Getenv("VCAP_APP_PORT"); port != "" {
		*flagPort = port
	}
	if wurl := os.Getenv("WMT_URL"); wurl != "" {
		*flagMongo = wurl
	}
	if wusr := os.Getenv("WMT_USER"); wusr != "" {
		*flagWatsonUser = wusr
	}
	if wpwd := os.Getenv("WMT_PWD"); wpwd != "" {
		*flagWatsonPwd = wpwd
	}
	if m := os.Getenv("MONGOLAB_URI"); m != "" {
		*flagMongo = m
	}
	session, err := mgo.Dial(*flagMongo)
	if err != nil {
		log.Fatalf("%s:%s", *flagMongo, err)
	}
	defer session.Close()
	server := &server{
		dbSession: session,
		dbName:    session.DB("").Name,
		wc: &watsonMTClient{
			c:           http.DefaultClient,
			apiEndpoint: *flagWatsonURL,
			apiUser:     *flagWatsonUser,
			apiPwd:      *flagWatsonPwd,
		},
	}
	http.HandleFunc("/dkv/", server.storageHandler)
	http.HandleFunc("/dkv/translate/", server.translateHandler)
	log.Fatal(http.ListenAndServe(*flagIP+":"+*flagPort, nil))
}
