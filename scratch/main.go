package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	layer := "75"
	if len(os.Args) > 1 {
		layer = os.Args[1]
	}
	url := "https://gis.tucsonaz.gov/arcgis/rest/services/PublicMaps/PermitsCode/MapServer/" + layer + "/query?where=1%3D1&returnCountOnly=true&f=json"
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
