package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/robfig/cron/v3"
)

type Job interface {
	Run()
}

type PrintJob struct {
	message string
}

type StoreTimestampJob struct {
	dbw *DBWrapper
}

type DBWrapper struct {
	db    *sql.DB
	mutex sync.RWMutex
}

func (dbw *DBWrapper) ReadData(query string, args ...interface{}) (*sql.Rows, error) {
	dbw.mutex.RLock()
	defer dbw.mutex.RUnlock()

	return dbw.db.Query(query, args...)
}

func (dbw *DBWrapper) WriteData(query string, args ...interface{}) (sql.Result, error) {
	dbw.mutex.Lock()
	defer dbw.mutex.Unlock()

	return dbw.db.Exec(query, args...)
}

func (p *PrintJob) Run() {
	fmt.Println(p.message, "at: ", time.Now())
}

func (p *StoreTimestampJob) Run() {
	currentTime := time.Now()

	if _, err := p.dbw.WriteData("INSERT INTO timestamps (timestamp) VALUES (?)", currentTime); err != nil {
		log.Printf("Failed to insert timestamp: %v", err)
	}

	fmt.Println("Timestamp inserted:", currentTime)
}

func InitStoreTimestampJob(dbw *DBWrapper) *StoreTimestampJob {
	job := &StoreTimestampJob{dbw: dbw}

	return job
}

func initDatabase(databasePath string) (*DBWrapper, error) {
	fmt.Println("Loading database...")
	db, err := sql.Open("sqlite3", databasePath)

	if err != nil {
		return nil, err
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS timestamps (
		id INTEGER PRIMARY KEY,
		timestamp DATETIME
	);
	`

	_, err = db.Exec(createTableQuery)

	if err != nil {
		return nil, err
	}

	fmt.Println("Ran migrations...")

	return &DBWrapper{
		db: db,
	}, nil

}

func handleForceClose(sigs chan os.Signal) {
	sig := <-sigs

	log.Printf("Received signal: %s Closing everything...", sig)

	os.Exit(0)
}

func initSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go handleForceClose(sigs)
}

type Timestamp struct {
	ID        int
	Timestamp time.Time
}

func readTimestamps(dbw *DBWrapper) ([]Timestamp, error) {
	rows, err := dbw.ReadData("SELECT * FROM timestamps")

	if err != nil {
		fmt.Println(err)
	}

	defer rows.Close()

	var timestamps []Timestamp

	for rows.Next() {
		var t Timestamp
		// err := rows.Scan(&id, &timestamp)
		if err := rows.Scan(&t.ID, &t.Timestamp); err != nil {
			return nil, err
		}

		timestamps = append(timestamps, t)
	}

	return timestamps, rows.Err()
}

func main() {
	initSignals()

	dbw, dbError := initDatabase("./data.db")

	if dbError != nil {
		log.Fatal("Error initialized the database", dbError)
	}

	defer dbw.db.Close()

	c := cron.New(cron.WithSeconds())

	job := InitStoreTimestampJob(dbw)

	_, err := c.AddJob("0 * * * * *", job)

	if err != nil {
		fmt.Println("Error scheduling task", err)
	}

	c.Start()

	timestamps, err := readTimestamps(dbw)

	if err != nil {
		log.Println("Error fetching timestamps:", err)
		return
	}

	for _, t := range timestamps {
		fmt.Printf("ID: %d, Timestamp: %s\n", t.ID, t.Timestamp)
	}

	//infinite loop to keep program running
	select {}
}
