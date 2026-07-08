package plugins

import "context"

type Params map[string]string

type Result struct {
Success  bool   `json:"success"`
Message  string `json:"message"`
Metadata Params `json:"metadata,omitempty"`
}

type Plugin interface {
Name() string
Describe() string
Run(ctx context.Context, target string, params Params) (Result, error)
}

var registry = map[string]Plugin{}

func Register(p Plugin) {
registry[p.Name()] = p
}

func Get(name string) (Plugin, bool) {
p, ok := registry[name]
return p, ok
}

func List() []string {
names := make([]string, 0, len(registry))
for name := range registry {
names = append(names, name)
}
return names
}
