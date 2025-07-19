package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"time"
	"log"
	"os"

	"github.com/lib/pq"
)

type Config struct {
	PGuri   string
	Channel string
	Sync    time.Duration
	Idle    time.Duration
}

type Counter struct {
	MessageId int `json:"messageid"`
	Rts       int `json:"rts"`
	Favs      int `json:"favs"`
}

type Cache map[int]*Counter

const sync_delay = 10 * time.Second
const idle_delay = 30 * time.Second
const scrn_width = 65
const min_reconn = 10 * time.Second
const max_reconn = time.Minute

func main() {
	conf := processFlags()
	log.SetOutput(os.Stdout)

	log.Printf("Connecting to %s… ", conf.PGuri)
	db, err := sql.Open("postgres", conf.PGuri)
	if err != nil {
		log.Printf("Failed to connect to '%s': %s", conf.PGuri, err)
		os.Exit(1)
	}
	defer db.Close()

	reportErr := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Failed to start listener: %s", err)
			os.Exit(1)
		}
	}

	//
	// First, LISTEN to incoming events
	//
	listener := pq.NewListener(conf.PGuri, min_reconn, max_reconn, reportErr)
	err = listener.Listen(conf.Channel)
	if err != nil {
		log.Printf(
			"Failed to LISTEN to channel '%s': %s",
			conf.Channel, err)
		panic(err)
	}
	log.Printf(
		"Listening to notifications on channel \"%s\"",
		conf.Channel)

	//
	// Second, initialize the cache with the current values from the
	// base table, only then proceed to process the notifications.
	//
	cache, err := initCache(db)

	if err != nil {
		log.Printf("Error initializing cache")
		panic(err)
	}
	log.Printf(
		"Cache initialized with %d entries.",
		len(cache))

	//
	// Third, grab notifications and process them by updating the
	// counters that changed.
	//
	log.Println("Start processing notifications, waiting for events…")

	reset := time.Now()
	for {
		waitForNotification(listener, cache, conf.Idle)

		if time.Since(reset) >= conf.Sync {
			err := materialize(db, cache)

			// reset the cache *unless* there was an error!
			if err == nil {
				cache = make(Cache)
				reset = time.Now()
			}
		}
	}
}

func processFlags() *Config {
	var conninfo string = "postgres:///yesql?sslmode=disable"
	var channel string = "tweet.activity"

	syncPtr := flag.Int("sync", int(sync_delay.Seconds()),
		"Sync cache every SYNC seconds")
	flag.StringVar(&conninfo, "pguri", conninfo,
		"PostgreSQL connection string")
	flag.StringVar(&channel, "channel", channel,
		"LISTEN to this channel")

	flag.Parse()

	sync := time.Duration(*syncPtr) * time.Second
	idle := sync / 4

	return &Config{conninfo, channel, sync, idle}
}

func waitForNotification(l *pq.Listener, cache Cache, timeout time.Duration) {
	select {
	case n := <-l.Notify:
		var c Counter
		err := json.Unmarshal([]byte(n.Extra), &c)

		if err != nil {
			log.Printf("Failed to parse '%s': %s", n.Extra, err)
		} else {
			log.Printf("Received event: %s", n.Extra)
			updateCounter(cache, c)
		}

	case <-time.After(timeout):
		return
	}
}

func initCache(db *sql.DB) (Cache, error) {
	q := "select messageid, rts, favs from tweet.message_with_counters;"

	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	cache := make(Cache)

	for rows.Next() {
		c := Counter{}
		err := rows.Scan(&c.MessageId, &c.Rts, &c.Favs)

		if err != nil {
			return nil, err
		}
		cache[c.MessageId] = &c
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return cache, nil
}

func materialize(db *sql.DB, cache Cache) error {
	if len(cache) == 0 {
		return nil
	}
	log.Printf("Materializing %d events from memory", len(cache))

	q := `
with rec as
 (
   select rec.*
     from json_each($1) as t,
          json_populate_record(null::twcache.counters, value) as rec
 )
 insert into twcache.counters(messageid, rts, favs)
      select messageid, rts, favs
        from rec
 on conflict (messageid)
   do update
         set rts  = counters.rts + excluded.rts,
             favs = counters.favs + excluded.favs
       where counters.messageid = excluded.messageid
`
	js, err := json.Marshal(cache)
	if err != nil {
		log.Printf("Error while materializing cache: %s", err)
		return err
	}

	_, err = db.Query(q, js)
	if err != nil {
		log.Printf("Error materliazing cache: %s", err)
		return err
	}
	return nil
}

func updateCounter(cache Cache, c Counter) {
	_, found := cache[c.MessageId]

	if found {
		cache[c.MessageId].Rts += c.Rts
		cache[c.MessageId].Favs += c.Favs
	} else {
		cache[c.MessageId] = &c
	}
}
