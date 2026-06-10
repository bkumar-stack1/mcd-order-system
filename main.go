package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var logger *log.Logger

func initLogger() func() {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("ログディレクトリの作成に失敗しました: %v", err)
	}

	logPath := filepath.Join(logDir, "order.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("ログファイルのオープンに失敗しました: %v", err)
	}

	// 標準出力とファイル出力の双方に出力
	multiWriter := io.MultiWriter(os.Stdout, file)
	logger = log.New(multiWriter, "", log.LstdFlags)

	return func() {
		file.Close()
	}
}

func main() {
	closeLog := initLogger()
	defer closeLog()

	logger.Println("[INFO] アプリケーションを起動しています...")

	// データベース初期化
	if err := InitDB(); err != nil {
		logger.Fatalf("[ERROR] データベースの初期化に失敗しました: %v", err)
	}
	defer CloseDB()

	// Go 1.22+ 標準マルチプレクサ（Enhanced routing）の活用
	mux := http.NewServeMux()

	// 3.1 注文管理機能
	mux.HandleFunc("POST /api/orders", handleCreateOrder)
	mux.HandleFunc("GET /api/orders", handleListOrders)
	mux.HandleFunc("GET /api/orders/{orderNo}", handleGetOrderDetail)
	mux.HandleFunc("PUT /api/orders/{orderNo}/status", handleUpdateOrderStatus)

	// 3.2 フロント掲示板機能
	mux.HandleFunc("POST /api/board", handleBoardAction)

	// 3.3 厨房機能
	mux.HandleFunc("POST /api/kitchen", handleKitchenAction)

	// 全てのエンドポイントを包括するCORSミドルウェアの適用
	serverAddr := "0.0.0.0:8080"
	server := &http.Server{
		Addr:    serverAddr,
		Handler: corsMiddleware(mux),
	}

	// グレースフルシャットダウンの実装
	go func() {
		logger.Printf("[INFO] サーバーがポート %s で起動しました...", serverAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("[ERROR] サーバーの起動に失敗しました: %v", err)
		}
	}()

	// シグナル待機
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Println("[INFO] サーバーを停止しています...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("[ERROR] サーバーの強制停止: %v", err)
	}

	logger.Println("[INFO] サーバーが正常に終了しました。")
}