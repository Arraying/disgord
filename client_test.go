package disgord

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/andersfylling/disgord/websocket"
)

func TestClient_Once(t *testing.T) {
	c, err := NewClient(&Config{
		BotToken: "testing",
	})
	if err != nil {
		panic(err)
	}

	dispatcher := c.evtDispatch
	if len(dispatcher.listenOnceOnly) > 0 {
		t.Errorf("expected dispatch to have 0 listeners. Got %d", len(dispatcher.listenOnceOnly))
	}

	wg := sync.WaitGroup{}
	c.Once(EventMessageCreate, func() {
		wg.Done()
	})
	if len(dispatcher.listenOnceOnly) != 1 {
		t.Errorf("expected dispatch to have 1 listener. Got %d", len(dispatcher.listenOnceOnly))
	}
	wg.Add(1) // only run once

	// trigger the handler
	dispatcher.triggerHandlers(nil, EventMessageCreate, c, nil)
	if len(dispatcher.listenOnceOnly) > 0 {
		t.Errorf("expected dispatch to have 0 listeners. Got %d", len(dispatcher.listenOnceOnly))
	}

	// trigger the handler, again
	dispatcher.triggerHandlers(nil, EventMessageCreate, c, nil)
	if len(dispatcher.listenOnceOnly) > 0 {
		t.Errorf("expected dispatch to have 0 listeners. Got %d", len(dispatcher.listenOnceOnly))
	}

	wg.Wait()
	// if wg.Done() is called more than once, we get a panic.

	// TODO: add a timeout
}

func TestClient_On(t *testing.T) {
	c, err := NewClient(&Config{
		BotToken: "testing",
	})
	if err != nil {
		panic(err)
	}

	dispatcher := c.evtDispatch
	if len(dispatcher.listeners) > 0 {
		t.Errorf("expected dispatch to have 0 listeners. Got %d", len(dispatcher.listeners))
	}

	wg := sync.WaitGroup{}
	c.On(EventMessageCreate, func() {
		wg.Done()
	})
	if len(dispatcher.listeners) != 1 {
		t.Errorf("expected dispatch to have 1 listener. Got %d", len(dispatcher.listeners))
	}
	wg.Add(2)

	// trigger the handler twice
	dispatcher.triggerHandlers(nil, EventMessageCreate, c, nil)
	dispatcher.triggerHandlers(nil, EventMessageCreate, c, nil)
	dispatcher.triggerHandlers(nil, EventReady, c, nil)
	wg.Wait()

	// TODO: add a timeout
}

// TestClient_System looks for crashes when the DisGord system starts up.
// the websocket logic is excluded to avoid crazy rewrites. At least, for now.
func TestClient_System(t *testing.T) {
	c, err := NewClient(&Config{
		BotToken: "testing",
	})
	if err != nil {
		panic(err)
	}

	c.shardManager.evtChan = make(chan *websocket.Event, 1)
	c.shardManager.shards = append(c.shardManager.shards, &WSShard{})
	input := c.shardManager.evtChan
	c.setupConnectEnv()

	var files []string

	root := "testdata/phases/startup-smooth-1"
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := range files {
		if files[i] == root {
			files[i] = files[len(files)-1]
			files = files[:len(files)-1]
			break
		}
	}
	sort.Slice(files, func(i, j int) bool {
		starti := strings.Split(files[i][len(root+"/"):], "_")
		startj := strings.Split(files[j][len(root+"/"):], "_")

		if _, err := strconv.Atoi(starti[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := strconv.Atoi(startj[0]); err != nil {
			t.Fatal(err)
		}

		a, _ := strconv.Atoi(starti[0])
		b, _ := strconv.Atoi(startj[0])

		return a < b
	})
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		p := &struct {
			E string          `json:"t"`
			D json.RawMessage `json:"d"`
		}{}
		err = json.Unmarshal(data, p)
		if err != nil {
			t.Fatal(err)
		}

		// ignore non-event-type packets
		if p.E == "" {
			continue
		}

		input <- &websocket.Event{
			Name: p.E,
			Data: p.D,
		}
	}

	// TODO: race - / don't have another way to "sync" the go routines
	//if _, err = c.cache.GetGuild(244200618854580224); err != nil {
	//	t.Error(err)
	//}

	//wg := sync.WaitGroup{}
	//wg.Add(1)
	//c.On(event.GuildMembersChunk, func(s Session, evt *GuildMembersChunk) {
	//	var msg string
	//	for i := range evt.Members {
	//		if evt.Members[i].User == nil {
	//			msg = fmt.Sprintf("expected user in member to not be nil. Got %+v", evt.Members[i])
	//			break
	//		}
	//	}
	//
	//	if msg != "" {
	//		t.Error(msg)
	//	}
	//	wg.Done()
	//})
	//wg.Wait()

	// cleanup
	close(c.evtDispatch.shutdown)
	close(c.shutdownChan)
}

func TestInternalStateHandlers(t *testing.T) {
	c, err := NewClient(&Config{
		BotToken: "testing",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.shardManager.shards = append(c.shardManager.shards, &WSShard{}) // don't remove

	id := Snowflake(123)

	if len(c.GetConnectedGuilds()) != 0 {
		t.Errorf("expected no guilds to have been added yet. Got %d, wants %d", len(c.GetConnectedGuilds()), 0)
	}

	c.handlerAddToConnectedGuilds(c, &GuildCreate{
		Guild: NewPartialGuild(id),
	})
	if len(c.GetConnectedGuilds()) != 1 {
		t.Errorf("expected one guild to have been added. Got %d, wants %d", len(c.GetConnectedGuilds()), 1)
	}

	c.handlerAddToConnectedGuilds(c, &GuildCreate{
		Guild: NewPartialGuild(id),
	})
	if len(c.GetConnectedGuilds()) != 1 {
		t.Errorf("Adding the same guild should not create another entry. Got %d, wants %d", len(c.GetConnectedGuilds()), 1)
	}

	c.handlerRemoveFromConnectedGuilds(c, &GuildDelete{
		UnavailableGuild: &GuildUnavailable{
			ID: 9999,
		},
	})
	if len(c.GetConnectedGuilds()) != 1 {
		t.Errorf("Removing a unknown guild should not affect the internal state. Got %d, wants %d", len(c.GetConnectedGuilds()), 1)
	}

	c.handlerRemoveFromConnectedGuilds(c, &GuildDelete{
		UnavailableGuild: &GuildUnavailable{
			ID: id,
		},
	})
	if len(c.GetConnectedGuilds()) != 0 {
		t.Errorf("Removing a connected guild should affect the internal state. Got %d, wants %d", len(c.GetConnectedGuilds()), 0)
	}
}
