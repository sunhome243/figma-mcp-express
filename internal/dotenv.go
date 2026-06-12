package internal

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv reads KEY=VALUE lines from the file at path and exports them as
// environment variables — but ONLY for keys not already present (the real
// process environment always wins, so an explicit `export FIGMA_TOKEN=...`
// overrides the file). A missing file is a no-op (returns 0, nil).
//
// Blank lines and lines starting with '#' are skipped. An optional leading
// "export " is tolerated. Surrounding single/double quotes around the value
// are stripped. Values are never logged by this function.
func LoadDotEnv(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[:eq]), "export "))
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, present := os.LookupEnv(key); present {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return count, err
		}
		count++
	}
	return count, sc.Err()
}
