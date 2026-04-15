package main

func process(records []Record) []string {
	var out []string
	for _, r := range records {
		out = append(out, r.Name)
	}
	return out
}
