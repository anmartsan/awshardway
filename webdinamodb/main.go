package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/julienschmidt/httprouter"
)

type usuario struct {
	// pass string `json:"pass" dynamodbav:"pass"`
	// user string `json:"user" dynamodbav:"user"`
	Pass string `json:"pass"`
	User string `json:"user"`
}
type user struct {
	Username string
	Password []byte
	ID       string
}

var tpl *template.Template
var users = map[string]user{}
var idUsers = map[string]user{}
var sess *session.Session

func queryusuarios(db *dynamodb.DynamoDB, name string, p string) *dynamodb.GetItemOutput {

	input := &dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"user": {
				S: aws.String(name),
			},
		},
		TableName: aws.String("usuarios"),
	}
	result, err := db.GetItem(input)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err.Error())

	}

	return result
}

func main() {

	//AWS sessions
	sess = session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	}))
	//AWS dinamodb
	db := dynamodb.New(sess)

	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))

	f, err := os.OpenFile("logfile.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0664)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	log.SetOutput(f)

	loadUsers()

	router := httprouter.New()
	router.GET("/", index(db))
	router.GET("/login", loginPage)
	router.POST("/login", login(db))
	router.GET("/logout", logout)
	router.GET("/create", createPage)
	router.POST("/create", create(db))
	go func() {
		err = http.ListenAndServe(":9000", router)
		if err != nil {
			panic(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	<-c
	saveUsers()
}

func create(s *dynamodb.DynamoDB) httprouter.Handle {
	return func(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		name := req.FormValue("username")
		p := req.FormValue("password")
		if len(name) < 3 || len(p) < 3 {
			http.Redirect(res, req, "/create?msg=Requires longer attributes", http.StatusSeeOther)
			return
		}
		usuarioDb := usuario{
			Pass: p,
			User: name,
		}

		usuarioMap, err := dynamodbattribute.MarshalMap(usuarioDb)
		if err != nil {
			panic("Cannot marshal usuario into AttributeValue map")
		}

		params := &dynamodb.PutItemInput{
			TableName: aws.String("usuarios"),
			Item:      usuarioMap,
		}
		resp, err := s.PutItem(params)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err.Error())
			return
		}
		fmt.Println(resp)

		// http.SetCookie(res, &http.Cookie{
		// 	Name:  "login",
		// 	Value: id.String(),
		// })
		http.Redirect(res, req, "/", http.StatusSeeOther)
	}
}
func index(s *dynamodb.DynamoDB) httprouter.Handle {
	return func(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		var username string
		c, err := req.Cookie("login")
		if err == nil {
			id := c.Value
			u, ok := idUsers[id]
			if !ok {
				log.Printf("error getting logged in user with id %s\n", id)
			} else {
				username = u.Username
			}
		}
		fmt.Println(s)
		err = tpl.ExecuteTemplate(res, "index", username)
		if err != nil {
			http.Error(res, "Server Error", http.StatusInternalServerError)
			log.Printf("error running index template: %s\n", err.Error())
			return
		}
	}
}

func loadUsers() {
	var rdr io.Reader
	f, err := os.Open("users.json")
	if err != nil {
		rdr = strings.NewReader("{}")
	} else {
		defer f.Close()
		rdr = f
	}
	err = json.NewDecoder(rdr).Decode(&users)
	if err != nil {
		panic(err)
	}
	for _, u := range users {
		idUsers[u.ID] = u
	}
}

func saveUsers() {
	f, err := os.Create("users.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(users)
	if err != nil {
		panic(err)
	}
}

func loginPage(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := tpl.ExecuteTemplate(res, "login", req.FormValue("msg"))
	if err != nil {
		http.Error(res, "Server Error", http.StatusInternalServerError)
		log.Printf("error running login template: %s\n", err.Error())
		return
	}
}
func login(db *dynamodb.DynamoDB) httprouter.Handle {
	return func(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {

		usuarioDb := usuario{}
		name := req.FormValue("username")
		p := req.FormValue("password")
		result := queryusuarios(db, name, p)

		err := dynamodbattribute.UnmarshalMap(result.Item, &usuarioDb)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err.Error())
			return
		}

		if len(usuarioDb.User) == 0 {
			log.Printf("error logging in, no such user: %s\n", name)
			http.Redirect(res, req, "/login?msg=No such user", http.StatusSeeOther)
			return
		}
		if (len(usuarioDb.User) > 0) && usuarioDb.Pass == p {
			http.Redirect(res, req, "/", http.StatusSeeOther)

		} else {
			http.Redirect(res, req, "/login?msg=Incorrect password", http.StatusSeeOther)
		}

		// http.SetCookie(res, &http.Cookie{
		// 	Name:  "login",
		// 	Value: u.ID,
		// })

	}
}

func logout(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	http.SetCookie(res, &http.Cookie{
		Name:   "login",
		MaxAge: -1,
	})
	http.Redirect(res, req, "/", http.StatusSeeOther)
}

func createPage(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := tpl.ExecuteTemplate(res, "create", req.FormValue("msg"))
	if err != nil {
		http.Error(res, "Server Error", http.StatusInternalServerError)
		log.Printf("error running create template: %s\n", err.Error())
		return
	}
}

// func create(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
// 	name := req.FormValue("username")
// 	p := req.FormValue("password")
// 	if len(name) < 3 || len(p) < 3 {
// 		http.Redirect(res, req, "/create?msg=Requires longer attributes", http.StatusSeeOther)
// 		return
// 	}
// 	id, err := uuid.NewV4()
// 	if err != nil {
// 		http.Error(res, "Server Error", http.StatusInternalServerError)
// 		log.Printf("error generating uuid: %s\n", err.Error())
// 		return
// 	}
// 	hashPass, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
// 	if err != nil {
// 		http.Error(res, "Server Error", http.StatusInternalServerError)
// 		log.Printf("error hashing password: %s\n", err.Error())
// 		return
// 	}
// 	u := user{
// 		Username: name,
// 		Password: hashPass,
// 		ID:       id.String(),
// 	}
// 	users[name] = u
// 	idUsers[id.String()] = u
// 	http.SetCookie(res, &http.Cookie{
// 		Name:  "login",
// 		Value: id.String(),
// 	})
// 	http.Redirect(res, req, "/", http.StatusSeeOther)
// }
