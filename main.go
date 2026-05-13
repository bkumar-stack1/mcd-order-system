package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    os.MkdirAll("logs", os.ModePerm)

    InitDB()
    InitLogger()

    http.HandleFunc("/api/orders", OrdersHandler)
    http.HandleFunc("/api/orders/", OrderDetailHandler)

    fmt.Println("サーバー起動: http://localhost:8080")

    log.Fatal(http.ListenAndServe(":8080", nil))
}