package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/pkg/errors"
)

func LoadData(path string, data *Data) bool {
	path += ".json"
	fmt.Println("loading savefile:", path)
	b, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(errors.WithMessage(err, "loading savefile"))
	}
	fmt.Println("parsing savefile")
	err = json.Unmarshal(b, data)
	if err != nil {
		panic(errors.WithMessage(err, "parsing savefile"))
	}
	fmt.Println("loaded")
	return true
}

func SaveData(path string, data *Data) error {
	path += ".json"
	log.Println("saving metadata to", path)
	b, err := json.MarshalIndent(&data, "", "  ")

	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path, b, 0644)
	if err != nil {
		return err
	}
	log.Println("done.")
	return nil
}
