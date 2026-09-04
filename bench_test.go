package tomledit

import (
	"bytes"
	"fmt"
	"testing"

	burntsushi "github.com/BurntSushi/toml"
)

var benchInput = []byte(`# Application config
[server]
host = "localhost"
port = 8080
debug = false

[server.database]
host = "db.example.com"
port = 5432
name = "myapp"
max_connections = 100

[[products]]
name = "Widget"
price = 9.99
tags = ["sale", "featured"]

[[products]]
name = "Gadget"
price = 19.99
tags = ["new"]

[metadata]
created = 2024-01-15T10:30:00Z
version = "1.0.0"
`)

func BenchmarkParse(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(benchInput)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBytes(b *testing.B) {
	doc, err := Parse(benchInput)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = doc.Bytes()
	}
}

func BenchmarkGet(b *testing.B) {
	doc, err := Parse(benchInput)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.GetString("server.database.name")
	}
}

func BenchmarkSetExisting(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, _ := Parse(benchInput)
		doc.Set("server.host", "newhost")
		_ = doc.Bytes()
	}
}

func BenchmarkFormat(b *testing.B) {
	doc, err := Parse(benchInput)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = doc.Format()
	}
}

func BenchmarkParseLarge(b *testing.B) {
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&buf, "[[items]]\nname = \"item_%d\"\nvalue = %d\nenabled = %v\n\n", i, i*100, i%2 == 0)
	}
	input := buf.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Parse(input)
	}
}

// Shared struct for unmarshal benchmarks (matches benchInput TOML).
type benchConfig struct {
	Server struct {
		Host     string `toml:"host"`
		Port     int    `toml:"port"`
		Debug    bool   `toml:"debug"`
		Database struct {
			Host            string `toml:"host"`
			Port            int    `toml:"port"`
			Name            string `toml:"name"`
			Max_connections int    `toml:"max_connections"`
		} `toml:"database"`
	} `toml:"server"`
	Products []struct {
		Name  string   `toml:"name"`
		Price float64  `toml:"price"`
		Tags  []string `toml:"tags"`
	} `toml:"products"`
	Metadata struct {
		Created interface{} `toml:"created"`
		Version string      `toml:"version"`
	} `toml:"metadata"`
}

func BenchmarkUnmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Unmarshal[benchConfig](benchInput); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBurntSushiParse(b *testing.B) {
	s := string(benchInput)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m map[string]any
		if _, err := burntsushi.Decode(s, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBurntSushiUnmarshal(b *testing.B) {
	s := string(benchInput)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg benchConfig
		if _, err := burntsushi.Decode(s, &cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBurntSushiParseLarge(b *testing.B) {
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&buf, "[[items]]\nname = \"item_%d\"\nvalue = %d\nenabled = %v\n\n", i, i*100, i%2 == 0)
	}
	s := buf.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m map[string]any
		if _, err := burntsushi.Decode(s, &m); err != nil {
			b.Fatal(err)
		}
	}
}
