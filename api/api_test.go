package api_test

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/cfstras/wiki-api/data"

	"github.com/cfstras/wiki-api/api"
	"github.com/stretchr/testify/assert"
)

var port int

func TestMain(m *testing.M) {
	flag.Parse()
	var ret int
	defer os.Exit(ret)
	no := func(err error) {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	port = rand.Intn(1024) + 10000
	tmp, err := ioutil.TempDir(os.TempDir(), "gotest")
	no(err)
	defer func() {
		if err := recover(); err != nil {
			os.RemoveAll(tmp)
			fmt.Println("caught panic:", err)
			os.Exit(1)
		}
	}()
	defer func() { no(os.RemoveAll(tmp)) }()

	tmp += "/"
	file, err := os.Open("../testdata.tar")
	no(err)
	defer file.Close()
	tarfile := tar.NewReader(file)
	for {
		hdr, err := tarfile.Next()
		if err == io.EOF {
			break
		}
		no(err)
		to := tmp + hdr.Name
		if hdr.FileInfo().IsDir() {
			//fmt.Println("mkdir", to)
			no(os.MkdirAll(to, 0755))
			continue
		}
		//fmt.Println("extracting", to)
		content, err := ioutil.ReadAll(tarfile)
		no(err)
		no(ioutil.WriteFile(to, content, 0644))
	}

	go func() {
		err := api.Run(fmt.Sprintf(":%d", port), tmp+"wiki-test.git")
		no(err)
	}()
	// wait until ready
	for {
		c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			//fmt.Println("error testing:", err)
			time.Sleep(50 * time.Millisecond)
		} else {
			no(c.Close())
			break
		}
	}
	ret = m.Run()
}

type testCase struct {
	url, response string
}

// testRequest calls a URL and verifies the result matches what is expected.
func testRequest(t *testing.T, c testCase) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, c.url))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, c.response, string(body))
}

// TestGetFile verifies that files from the repository are correctly returned.
func TestGetFile(t *testing.T) {
	cases := []testCase{
		{"/main.md", `# Main

Welcome to the best Wiki _ever_ to be created!

---

**lol**.
`},
		{"/foo/foo.txt", "foo.txt\n"},
		{"/foo/bar/a.md", "foo/bar/a.md\n"},
		{"/foo/bar/baz/boo/x.md", "foo/bar/baz/boo/x.md\n"},
	}

	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			testRequest(t, c)
		})
	}
}

// TestGetListing tests the rendered response of directory listings.
// it also verifies the ordering of listings, and that both "/path" and "/path/"
// return the same result.
func TestGetListing(t *testing.T) {
	templateSource := string(data.MustAsset("indexOf.mustache"))
	template, err := mustache.ParseString(templateSource)
	assert.NoError(t, err)

	cases := []testCase{}

	res, err := template.Render(map[string]interface{}{
		"Path": "/",
		"Files": []map[string]interface{}{
			{"IsDir": true, "Name": "foo", "ID": "af780afe62a33918cc3868f992baf95ed89df45d"},
			{"IsDir": false, "Name": "main.md", "ID": "a58ad1f7cf02de3538fe4b6252dc049b9fdf698a"},
		}})
	assert.NoError(t, err)
	cases = append(cases, testCase{"/", res})

	res, err = template.Render(map[string]interface{}{
		"Path": "/foo/",
		"Files": []map[string]interface{}{
			{"IsDir": true, "Name": "bar", "ID": "f89102e8f7d3d0f2f4168b3ad300b902a3e90db6"},
			{"IsDir": false, "Name": "foo.txt", "ID": "7c6ded14ecffa0341f8dc68fb674d4ae26d34644"},
		}})
	assert.NoError(t, err)
	cases = append(cases, testCase{"/foo", res})
	cases = append(cases, testCase{"/foo/", res})

	res, err = template.Render(map[string]interface{}{
		"Path": "/foo/bar/",
		"Files": []map[string]interface{}{
			{"IsDir": false, "Name": "a.md", "ID": "29f793097574c57c748dbf83b25710bfa90f0505"},
			{"IsDir": true, "Name": "baz", "ID": "21be1b42bce2d050160f7a9b46ed8946de68e37e"},
		}})
	assert.NoError(t, err)
	cases = append(cases, testCase{"/foo/bar", res})
	cases = append(cases, testCase{"/foo/bar/", res})

	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			testRequest(t, c)
		})
	}
}
