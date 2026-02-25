package prompt

import "embed"

//go:embed templates/*
var templates embed.FS

// Template returns the content of an embedded template file.
func Template(name string) (string, error) {
	data, err := templates.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
