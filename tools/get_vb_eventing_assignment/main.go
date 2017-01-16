package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	host := os.Args[1]
	url := "http://" + host + ":9500/eventing/_design/ddoc1/_view/view1?stale=false"

	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	buf, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}

	var data map[string]interface{}
	err = json.Unmarshal(buf, &data)
	if err != nil {
		log.Fatal(err)
	}

	vbucketEventingNodeMap := make(map[string]map[string][]int)
	vbucketRequestingNodesMap := make(map[string][]int)

	rows, ok := data["rows"].([]interface{})
	if ok {
		for i := range rows {
			row := rows[i].(map[string]interface{})

			vbucket, _ := strconv.Atoi(strings.Split(row["id"].(string), "_")[3])
			viewKey := row["key"].([]interface{})
			currentOwner, workerId := viewKey[0].(string), viewKey[1].(string)
			newOwner := row["value"].(string)

			if _, ok := vbucketEventingNodeMap[currentOwner]; !ok && currentOwner != "" {
				vbucketEventingNodeMap[currentOwner] = make(map[string][]int)
				vbucketEventingNodeMap[currentOwner][workerId] = make([]int, 0)
			}

			if _, ok := vbucketRequestingNodesMap[newOwner]; !ok && newOwner != "" {
				vbucketRequestingNodesMap[newOwner] = make([]int, 0)
			}

			if currentOwner != "" && workerId != "" {
				vbucketEventingNodeMap[currentOwner][workerId] = append(
					vbucketEventingNodeMap[currentOwner][workerId], vbucket)
			}

			if newOwner != "" {
				vbucketRequestingNodesMap[newOwner] = append(
					vbucketRequestingNodesMap[newOwner], vbucket)
			}
		}

		fmt.Printf("\nvbucket curr owner:\n")
		for k1, _ := range vbucketEventingNodeMap {
			fmt.Printf("Producer node: %s\n", k1)
			for k2, _ := range vbucketEventingNodeMap[k1] {
				sort.Ints(vbucketEventingNodeMap[k1][k2])
				fmt.Printf("\tworkerId: %s\n\tlen: %d\n\tv: %v\n", k2, len(vbucketEventingNodeMap[k1][k2]),
					vbucketEventingNodeMap[k1][k2])
			}
		}

		fmt.Printf("\nvbucket requesting owner:\n")
		for k, _ := range vbucketRequestingNodesMap {
			sort.Ints(vbucketRequestingNodesMap[k])
			fmt.Printf("k: %s len: %d v: %v\n", k, len(vbucketRequestingNodesMap[k]),
				vbucketRequestingNodesMap[k])
		}
	}

}
