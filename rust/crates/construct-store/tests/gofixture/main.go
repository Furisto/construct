package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/memory"
	_ "github.com/furisto/construct/backend/memory/runtime"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

func main() {
	ctx := context.Background()
	client, err := memory.Open(dialect.SQLite, "file:"+os.Args[2]+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	check(err)
	defer client.Close()
	check(client.Schema.Create(ctx))
	db, err := sql.Open("sqlite", os.Args[2])
	check(err)
	defer db.Close()
	switch os.Args[1] {
	case "create":
		create(ctx, client)
		fmt.Println("Go fixture created: all six entities")
		return
	case "verify":
		verify(ctx, client, db)
		fmt.Println("Go verified Rust records and byte-identical JSON: all six entities")
		return
	}
	rows, err := db.Query("SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY type DESC, name")
	check(err)
	defer rows.Close()
	var schema []string
	for rows.Next() {
		var s string
		check(rows.Scan(&s))
		schema = append(schema, s)
	}
	check(rows.Err())
	b, err := json.Marshal(schema)
	check(err)
	fmt.Println(string(b))
}

func id(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", n)) }

func create(ctx context.Context, c *memory.Client) {
	t, err := time.Parse(time.RFC3339Nano, "2025-02-03T04:05:06.123456789Z")
	check(err)
	u := t.Add(time.Second)
	_, err = c.ModelProvider.Create().SetID(id(1)).SetName("provider <&>").SetProviderType(types.ModelProviderTypeAnthropic).SetURL("https://example.test").SetSecret([]byte{0, 1, 127, 128, 255}).SetEnabled(true).SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Model.Create().SetID(id(2)).SetName("model").SetModelProviderID(id(1)).SetContextWindow(123456).SetCapabilities([]types.ModelCapability{types.ModelCapabilityImage, types.ModelCapabilityPromptCache}).SetInputCost(1.25).SetOutputCost(2.5).SetCacheWriteCost(0.75).SetCacheReadCost(0.125).SetEnabled(true).SetAlias("alias").SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Agent.Create().SetID(id(3)).SetName("agent").SetDescription("description").SetInstructions("instructions <&>\u2028").SetBuiltin(true).SetModelID(id(2)).SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Task.Create().SetID(id(4)).SetProjectDirectory("/workspace").SetInputTokens(101).SetOutputTokens(102).SetCacheWriteTokens(103).SetCacheReadTokens(104).SetCost(1.25).SetTurns(9).SetToolUses(map[string]int64{"z": 2, "a<&>": 1}).SetDesiredPhase(types.TaskPhaseRunning).SetPhase(types.TaskPhaseAwaiting).SetDescription("task").SetAgentID(id(3)).SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Message.Create().SetID(id(5)).SetSource(types.MessageSourceAssistant).SetContent(&types.MessageContent{Blocks: []types.MessageBlock{{Kind: types.MessageBlockKindText, Payload: "hello <&>\u2028"}, {Kind: types.MessageBlockKindToolCall, Payload: "{\"name\":\"shell\"}"}}}).SetUsage(&types.MessageUsage{InputTokens: 11, OutputTokens: 12, CacheWriteTokens: 13, CacheReadTokens: 14, Cost: 0.125}).SetProcessedTime(u).SetTaskID(id(4)).SetAgentID(id(3)).SetModelID(id(2)).SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Token.Create().SetID(id(6)).SetName("token").SetType(types.TokenTypeAPIToken).SetTokenHash([]byte{9, 0, 255}).SetDescription("token description").SetExpiresAt(t.Add(24 * time.Hour)).SetCreateTime(t).SetUpdateTime(u).Save(ctx)
	check(err)
	_, err = c.Agent.Create().SetID(id(7)).SetName("monotonic-time").SetInstructions("default timestamps from time.Now").Save(ctx)
	check(err)
	_, err = c.Agent.Create().SetID(id(8)).SetName("offset-time").SetInstructions("non-UTC time").SetCreateTime(t.In(time.FixedZone("TEST", 19800))).SetUpdateTime(u.In(time.FixedZone("TEST", 19800))).Save(ctx)
	check(err)
}

func verify(ctx context.Context, c *memory.Client, db *sql.DB) {
	for _, n := range []int{7, 8} {
		a, err := c.Agent.Get(ctx, id(n))
		check(err)
		b, err := c.Agent.Get(ctx, id(n+100))
		check(err)
		if b.Name != "rust-"+a.Name {
			panic("Rust time fixture name")
		}
		b.ID, b.Name = a.ID, a.Name
		equal(a, b)
	}
	p, err := c.ModelProvider.Get(ctx, id(1))
	check(err)
	q, err := c.ModelProvider.Get(ctx, id(101))
	check(err)
	q.ID = p.ID
	equal(p, q)
	m, err := c.Model.Get(ctx, id(2))
	check(err)
	n, err := c.Model.Get(ctx, id(102))
	check(err)
	n.ID, n.ModelProviderID = m.ID, m.ModelProviderID
	equal(m, n)
	a, err := c.Agent.Get(ctx, id(3))
	check(err)
	b, err := c.Agent.Get(ctx, id(103))
	check(err)
	if b.Name != "rust-agent" {
		panic("Rust agent name")
	}
	b.ID, b.ModelID = a.ID, a.ModelID
	b.Name = a.Name
	equal(a, b)
	t, err := c.Task.Get(ctx, id(4))
	check(err)
	v, err := c.Task.Get(ctx, id(104))
	check(err)
	v.ID, v.AgentID = t.ID, t.AgentID
	equal(t, v)
	x, err := c.Message.Get(ctx, id(5))
	check(err)
	y, err := c.Message.Get(ctx, id(105))
	check(err)
	y.ID, y.TaskID, y.AgentID, y.ModelID = x.ID, x.TaskID, x.AgentID, x.ModelID
	equal(x, y)
	k, err := c.Token.Get(ctx, id(6))
	check(err)
	l, err := c.Token.Get(ctx, id(106))
	check(err)
	if l.Name != "rust-token" || !bytes.Equal(l.TokenHash, []byte{10, 0, 255}) {
		panic("Rust token fields")
	}
	l.ID = k.ID
	l.Name, l.TokenHash = k.Name, k.TokenHash
	equal(k, l)
	for _, pair := range []struct {
		table, column string
		goID, rustID  int
	}{{"messages", "content", 5, 105}, {"messages", "usage", 5, 105}, {"tasks", "tool_uses", 4, 104}, {"models", "capabilities", 2, 102}} {
		var goJSON, rustJSON []byte
		query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", pair.column, pair.table)
		check(db.QueryRow(query, id(pair.goID).String()).Scan(&goJSON))
		check(db.QueryRow(query, id(pair.rustID).String()).Scan(&rustJSON))
		if !bytes.Equal(goJSON, rustJSON) {
			panic("JSON bytes differ: " + pair.column)
		}
	}
}

func equal(a, b any) {
	x, y := reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem()
	for i := 0; i < x.NumField(); i++ {
		field := x.Type().Field(i)
		if !field.IsExported() || field.Name == "Edges" {
			continue
		}
		left, right := x.Field(i).Interface(), y.Field(i).Interface()
		if t, ok := left.(time.Time); ok {
			if !t.Equal(right.(time.Time)) {
				panic(fmt.Sprintf("Go/Rust time differs: %T.%s", a, field.Name))
			}
		} else if !reflect.DeepEqual(left, right) {
			panic(fmt.Sprintf("Go/Rust scalar differs: %T.%s", a, field.Name))
		}
	}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
