mongostore
==========
[![CodeQL](https://github.com/laziness-coders/mongostore/actions/workflows/codeql.yml/badge.svg)](https://github.com/laziness-coders/mongostore/actions/workflows/codeql.yml)
[![Run Tests](https://github.com/laziness-coders/mongostore/actions/workflows/go.yml/badge.svg)](https://github.com/laziness-coders/mongostore/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/laziness-coders/mongostore)](https://goreportcard.com/report/github.com/laziness-coders/mongostore)
[![GoDoc](https://godoc.org/github.com/laziness-coders/mongostore?status.svg)](https://godoc.org/github.com/laziness-coders/mongostore)
[![codecov](https://codecov.io/gh/laziness-coders/mongostore/graph/badge.svg?token=FYUKE38KDS)](https://codecov.io/gh/laziness-coders/mongostore)

[Gorilla's Session](http://www.gorillatoolkit.org/pkg/sessions) store implementation with MongoDB

## Requirements

Depends on the [mgo](https://github.com/kidstuff/mongostore) library.

## Installation

For the latest go version, run:

    go get github.com/laziness-coders/mongostore

For the go version 1.20 and under, run:

    go get github.com/laziness-coders/mongostore@v0.0.6

## Documentation

Available on [godoc.org](https://www.godoc.org/github.com/laziness-coders/mongostore).

### Example
```go
    func foo(rw http.ResponseWriter, req *http.Request) {
        // Fetch new store..
    	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
    	if err != nil {
    		panic(err)
    	}
    	
    	if err := client.Connect(context.Background()); err != nil {
    		panic(err)
    	}
    	defer client.Disconnect(context.Background())

        // Get a session.
        store := NewMongoStore(
            client.Database("test").Collection("test_session"),
            3600,
            false,
            []byte("secret-key"),
        )

        // Add a value.
        session.Values["foo"] = "bar"

        // Save.
        if err = sessions.Save(req, rw); err != nil {
            log.Printf("Error saving session: %v", err)
        }

        fmt.Fprintln(rw, "ok")
    }
```
