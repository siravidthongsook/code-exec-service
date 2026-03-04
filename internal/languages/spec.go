package languages

import "strings"

type Spec struct {
	Compile string
	Run     string
}

var defaultSpecs = map[string]Spec{
	"python":     {Run: "python3 {src}"},
	"cpp":        {Compile: "g++ -std=c++17 -O2 -pipe {src} -o {exe}", Run: "{exe}"},
	"c":          {Compile: "gcc -std=c11 -O2 -pipe {src} -o {exe}", Run: "{exe}"},
	"js":         {Run: "node {src}"},
	"javascript": {Run: "node {src}"},
	"java":       {Compile: "javac {src}", Run: "java -cp {workdir} Main"},
	"go":         {Compile: "go build -o {exe} {src}", Run: "{exe}"},
}

func Lookup(lang string) (Spec, bool) {
	spec, ok := defaultSpecs[Normalize(lang)]
	return spec, ok
}

func Normalize(lang string) string {
	return strings.ToLower(strings.TrimSpace(lang))
}
