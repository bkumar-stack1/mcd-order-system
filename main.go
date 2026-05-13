package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// logsフォルダ作成
	_ = os.MkdirAll("logs", 0755)

	// DB初期化
	initDB()
	defer db.Close()

	// ルーティング (handler.goの関数名と一致させる)
	http.HandleFunc("/api/orders", OrdersHandler)
	http.HandleFunc("/api/orders/", OrderDetailHandler)

	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}