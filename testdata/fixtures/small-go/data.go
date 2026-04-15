package main

type Record struct {
	ID   int
	Name string
}

func getData() []Record {
	return []Record{{1, "test"}}
}
