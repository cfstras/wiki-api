package api_test

import (
	"archive/tar"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
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
		err := api.Run(fmt.Sprintf(":%d", port), tmp+"wiki-test.git", false)
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
	ret := m.Run()
	no(os.RemoveAll(tmp))
	os.Exit(ret)
}

type testCase struct {
	url, expected   string
	compareResponse func(t *testing.T, expected, actual string)
}

// testRequest calls a URL and verifies the result matches what is expected.
func testRequest(t *testing.T, c testCase) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, c.url))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bodyB, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)

	body := string(bodyB)
	if c.compareResponse != nil {
		c.compareResponse(t, c.expected, body)
	} else {
		assert.Equal(t, c.expected, body)
	}
}

// TestGetFile verifies that files from the repository are correctly returned.
func TestGetFile(t *testing.T) {
	compareJson := func(t *testing.T, expected, actual string) {
		var parsedExpected map[string]interface{}
		if !assert.NoError(t, json.Unmarshal([]byte(expected), &parsedExpected),
			"parsing expected json: "+expected) {
			t.FailNow()
		}

		var parsedActual map[string]interface{}
		if !assert.NoError(t, json.Unmarshal([]byte(actual), &parsedActual),
			"parsing actual json: "+actual) {
			t.FailNow()
		}

		assert.Equal(t, parsedExpected, parsedActual)
	}
	compareJson(t, "{}", "{}") // test compare function

	cases := []testCase{
		{url: "/main.md", expected: `# Main

Welcome to the best Wiki _ever_ to be created!

---

**lol**.
`},
		{url: "/foo/foo.txt", expected: "foo.txt\n"},
		{url: "/foo/bar/a.md", expected: "foo/bar/a.md\n"},
		{url: "/foo/bar/baz/boo/x.md", expected: "foo/bar/baz/boo/x.md\n"},

		{url: "/foo/foo.txt.json", compareResponse: compareJson, expected: `
{
	"Path": "/foo/foo.txt",
	"ID": "7c6ded14ecffa0341f8dc68fb674d4ae26d34644",
	"History": [
		{
			"Date": "2016-10-19T23:08:01+02:00",
			"CommitMsg": "revert everything",
			"Author": {
				"Name": "Claus Strasburger",
				"Email": "claus@strasburger.de"
			},
			"ID": "663a51383fc6fc6052a2570b9aff4c90a035305c"
		},
		{
			"ID": "2c35554157d56445d70ce121e4764f864a4c92bb",
			"Date": "2016-10-19T23:07:04+02:00",
			"CommitMsg": "move",
			"Author": {
				"Name": "Claus Strasburger",
				"Email": "claus@strasburger.de"
			}
		},
		{
			"ID": "94b931b4ecb3f461304dbf7a751b0c12cffaa9bf",
			"Date": "2016-10-19T23:04:32+02:00",
			"CommitMsg": "rewrite",
			"Author": {
				"Name": "Claus Strasburger",
				"Email": "claus@strasburger.de"
			}
		}
	]
}
		`},
		{url: "/foo/bar/baz/.json", compareResponse: compareJson, expected: `
{
	"Path": "/foo/bar/baz/",
	"ID": "21be1b42bce2d050160f7a9b46ed8946de68e37e",
	"History": [
		{
			"ID": "663a51383fc6fc6052a2570b9aff4c90a035305c",
			"Date": "2016-10-19T23:08:01+02:00",
			"CommitMsg": "revert everything",
			"Author": {
				"Name": "Claus Strasburger",
				"Email": "claus@strasburger.de"
			}
		}
	],
	"Files": [
		{
			"Name": "boo",
			"ID": "59f6de287017b034e5df4d1a5f4ad3986ad8d9c3",
			"IsDir": true
		}
	]
}
`},
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
	cases = append(cases, testCase{url: "/", expected: res})

	res, err = template.Render(map[string]interface{}{
		"Path": "/foo/",
		"Files": []map[string]interface{}{
			{"IsDir": true, "Name": ".."},
			{"IsDir": true, "Name": "bar", "ID": "f89102e8f7d3d0f2f4168b3ad300b902a3e90db6"},
			{"IsDir": false, "Name": "foo.txt", "ID": "7c6ded14ecffa0341f8dc68fb674d4ae26d34644"},
		}})
	assert.NoError(t, err)
	cases = append(cases, testCase{url: "/foo", expected: res})
	cases = append(cases, testCase{url: "/foo/", expected: res})

	res, err = template.Render(map[string]interface{}{
		"Path": "/foo/bar/",
		"Files": []map[string]interface{}{
			{"IsDir": true, "Name": ".."},
			{"IsDir": false, "Name": "a.md", "ID": "29f793097574c57c748dbf83b25710bfa90f0505"},
			{"IsDir": true, "Name": "baz", "ID": "21be1b42bce2d050160f7a9b46ed8946de68e37e"},
		}})
	assert.NoError(t, err)
	cases = append(cases, testCase{url: "/foo/bar", expected: res})
	cases = append(cases, testCase{url: "/foo/bar/", expected: res})

	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			testRequest(t, c)
		})
	}
}

type putTestCase struct {
	testTitle    string
	path         string
	headers      []string
	content      string
	responseCode int
}

func TestPut(t *testing.T) {
	cases := []putTestCase{
		{"+ Regular 1", "/testfile-1.md", []string{},
			"#TEST CONTENT\nray-yay-yay-yay", 200},
		{"+ Regular 2", "/testfile-1.txt", []string{},
			"wow.", 200},
		{"- .json forbidden", "/testfile-2.json", []string{},
			"wow.", 409},
		{"+ Last-Id null", "/otherfile.txt", []string{"Wiki-Last-Id", "null"},
			"wow.", 200},
		{"- Last-Id wrong", "/main.md", []string{"Wiki-Last-Id", "01234abcde"},
			"wow.", 409},
		{"+ Last-Id right", "/main.md", []string{"Wiki-Last-Id", "a58ad1f7cf02de3538fe4b6252dc049b9fdf698a"},
			"wow.", 200},

		{"+ folders", "/foo/bar/baz/b.md", []string{},
			"Test 2", 200},
		{"+ Existing Folder", "/new_folder/test.txt", []string{},
			"Test", 200},
		{"+ New Folder", "/new_folder/test.txt", []string{},
			"Test", 200},
		{"+ New Folder With Spaces", "/new folder/test.txt", []string{},
			"Test", 200},
		{"- Bad Folder 1", "/foo/../test.txt", []string{},
			"Test 2", 400},
		{"- Bad Folder 2", "/new folder/../test.txt", []string{},
			"Test 2", 400},
	}

	for _, c := range cases {
		t.Run(c.testTitle, func(t *testing.T) {
			testPutRequest(t, c)
		})
	}
}

// testPutRequest calls PUT on a URL and verifies the result matches what is
// expected.
func testPutRequest(t *testing.T, c putTestCase) {
	req, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("http://127.0.0.1:%d%s", port, c.path),
		strings.NewReader(c.content))
	assert.NoError(t, err)
	for i := 0; i < len(c.headers); i += 2 {
		req.Header.Add(c.headers[i], c.headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bodyB, err := ioutil.ReadAll(resp.Body)
	body := string(bodyB)
	assert.NoError(t, err, body)

	//assert.Equal(t, "", string(body))
	assert.Equal(t, c.responseCode, resp.StatusCode, body)

	if c.responseCode == 200 {
		checkCase := testCase{url: c.path, expected: c.content}
		testRequest(t, checkCase)
	}
}
