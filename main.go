package main

import (
	"flag"
	"fmt"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

// HTTP Server para implementar el "distributed hash table"
type server struct {
	dbSession *mgo.Session
	dbName    string
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

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func main() {
	flagIP := flag.String("ip", "127.0.0.1", "ip address")
	flagPort := flag.String("port", "6060", "tcp port")
	flagMongo := flag.String("mongo", "localhost:27017/dkv", "mongodb URL")
	flag.Parse()

	if ip := os.Getenv("VCAP_APP_HOST"); ip != "" {
		*flagIP = ip
	}
	if port := os.Getenv("VCAP_APP_PORT"); port != "" {
		*flagPort = port
	}
	session, err := mgo.Dial(*flagMongo)
	if err != nil {
		log.Fatalf("%s:%s", *flagMongo, err)
	}
	defer session.Close()
	server := &server{
		dbSession: session,
		dbName:    session.DB("").Name,
	}
	http.Handle("/dkv/", server)
	log.Fatal(http.ListenAndServe(*flagIP+":"+*flagPort, nil))
}
