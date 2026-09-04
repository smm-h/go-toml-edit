package tomledit_test

import (
	"fmt"

	"github.com/smm-h/go-toml-edit"
)

func ExampleParse() {
	doc, err := tomledit.Parse([]byte(`[server]
host = "localhost"
port = 8080
`))
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.GetString("server.host"))
	fmt.Println(doc.GetInt("server.port"))
	// Output:
	// localhost true
	// 8080 true
}

func ExampleDocument_Set() {
	doc, err := tomledit.Parse([]byte(`[server]
host = "localhost"
port = 8080
`))
	if err != nil {
		panic(err)
	}
	err = doc.Set("server.port", 9090)
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.GetInt("server.port"))
	// Output:
	// 9090 true
}

func ExampleDocument_SetCreate() {
	doc, err := tomledit.Parse([]byte(`title = "example"
`))
	if err != nil {
		panic(err)
	}
	// SetCreate auto-creates intermediate [database] table.
	err = doc.SetCreate("database.host", "db.example.com")
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.GetString("database.host"))
	// Output:
	// db.example.com true
}

func ExampleDocument_Delete() {
	doc, err := tomledit.Parse([]byte(`[server]
host = "localhost"
port = 8080
debug = true
`))
	if err != nil {
		panic(err)
	}
	err = doc.Delete("server.debug")
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.GetBool("server.debug"))
	// Output:
	// false false
}

func ExampleDocument_Format() {
	doc, err := tomledit.Parse([]byte(`name="Alice"
[server]
host="localhost"
port=8080
`))
	if err != nil {
		panic(err)
	}
	formatted := doc.Format(tomledit.WithIndentWidth(2))
	fmt.Print(string(formatted))
	// Output:
	// name = "Alice"
	//
	// [server]
	//   host = "localhost"
	//   port = 8080
}

func ExampleDocument_Key() {
	doc, err := tomledit.Parse([]byte(`[database]
host = "localhost"
port = 5432
`))
	if err != nil {
		panic(err)
	}
	host, ok := doc.Key("database").Key("host").String()
	fmt.Println(host, ok)

	port, ok := doc.Key("database").Key("port").Int()
	fmt.Println(port, ok)
	// Output:
	// localhost true
	// 5432 true
}

func ExampleUnmarshal() {
	type Config struct {
		Title  string `toml:"title"`
		Server struct {
			Host string `toml:"host"`
			Port int    `toml:"port"`
		} `toml:"server"`
	}
	var cfg Config
	err := tomledit.Unmarshal([]byte(`title = "My App"

[server]
host = "0.0.0.0"
port = 443
`), &cfg)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Title)
	fmt.Println(cfg.Server.Host)
	fmt.Println(cfg.Server.Port)
	// Output:
	// My App
	// 0.0.0.0
	// 443
}

func ExampleDocument_Walk() {
	doc, err := tomledit.Parse([]byte(`name = "example"

[server]
host = "localhost"
port = 8080
`))
	if err != nil {
		panic(err)
	}
	err = doc.Walk(func(path string, node tomledit.Node) error {
		fmt.Printf("%s = %v\n", path, node.(tomledit.Scalar).Value())
		return nil
	}, tomledit.WalkLeaves)
	if err != nil {
		panic(err)
	}
	// Output:
	// name = example
	// server.host = localhost
	// server.port = 8080
}

func ExampleDiff() {
	a, _ := tomledit.Parse([]byte(`name = "Alice"
age = 30
`))
	b, _ := tomledit.Parse([]byte(`name = "Bob"
email = "bob@example.com"
`))
	changes := tomledit.Diff(a, b)
	for _, c := range changes {
		fmt.Printf("%s: %s\n", c.Kind, c.Path)
	}
	// Output:
	// removed: age
	// added: email
	// modified: name
}

func ExampleDocument_Merge() {
	base, _ := tomledit.Parse([]byte(`[server]
host = "localhost"
`))
	defaults, _ := tomledit.Parse([]byte(`[server]
host = "0.0.0.0"
port = 8080
`))
	err := base.Merge(defaults)
	if err != nil {
		panic(err)
	}
	// host was already set, so it keeps "localhost".
	fmt.Println(base.GetString("server.host"))
	// port was missing, so it was added from defaults.
	fmt.Println(base.GetInt("server.port"))
	// Output:
	// localhost true
	// 8080 true
}
