package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"../internal"
	"../lib/support/client"
	"../lib/support/rpc"

	"strings"
	"path/filepath"
)

var server * rpc.ServerRemote

func main() {

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %v <server>\n", os.Args[0])
		os.Exit(1)
	}

	server = rpc.NewServerRemote(os.Args[1])

	var success string

	// authenticate user based on cookie value sent to server
	err := server.Call("authenticate", &success, getCookie())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error calling method authenticate: %v\n", err)
		return
	}

	c := Client{server}

	// check if authentication is success, spawn appropriate CLI
	if (strings.Contains(success, "false")) {
		if strings.Contains(success, "session expired"){
			fmt.Println("session expired")
		}
		// launches seperate login REPL before RunCLI REPL
		launchREPLs()
	} else {
		// goes straight to RunCLI REPL
		fmt.Println("logged in, welcome " + success)
		err := client.RunCLI(&c)
		if err != nil {
			fmt.Printf("fatal error: %v\n", err)
			os.Exit(1)
		}
	}
}

type Client struct {
	server *rpc.ServerRemote
}

/*
 * cdHandler() - launches access control REPL (RunCLI) only after successful login
 * 						in RunAuth() login REPL
 *
 * Parameters: none
 * Returns: none
 */
func launchREPLs() {
	c := Client{server}
	fmt.Println("please log in: \"login <username> <password>\" or sign up: \"signup <username> <password>\"")
	// login REPL
	client.RunAuth(&c)
	// access control REPL
	err := client.RunCLI(&c)
	if err != nil {
		fmt.Printf("fatal error: %v\n", err)
		os.Exit(1)
	}
}

/*
 * getCookie() - gets cookie for this client from bin
 *
 * Parameters: none
 * Returns: a string representing the cookie
 */
func getCookie() string {
	// get current working directory(absolute path) to be used to access cookie
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))

	// for testing change path to your bin
	if !strings.Contains(dir, "bin") {
		dir = "/gpfs/main/home/nwey/course/cs166/dropbox/s18-ptouma-nwey/updated_code/bin"
	}

	// attempt to get the cookie from the .cookie file
	b, err := ioutil.ReadFile(dir + "/cookie.txt")

	// convert to string
	var cookie string = ""
	if err == nil {
		cookie = string(b)
	}

	// trim whitespace
	cookie = strings.TrimSpace(cookie)
	return cookie
}

/*
 * Delete() - calls deleteHandler in server to delete user
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: user is deleted on server, cookie invalidated
 * Parameters: none
 * Returns: an error if request malfunctions
 */
func (c *Client) Delete() (err error) {
	var ret string
	// sends cookie as argument to handler
	err = c.server.Call("delete", &ret, getCookie())
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	fmt.Println("successfully deleted account")
	// relaunch login REPL for new login
	launchREPLs()
	return nil
}

/*
 * LogOut() - calls logoutHandler in server to logout of session
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: cookie is invalidated for session on server-side
 * Parameters: none
 * Returns: an error if request malfunctions
 */
func (c *Client) LogOut() (err error) {
	var ret string
	// sends cookie as argument to handler
	err = c.server.Call("logout", &ret, getCookie())
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	fmt.Println("logged out")
	// relaunch login REPL for new login
	launchREPLs()
	return nil
}

/*
 * SignUp() - calls signupHandler in server to sign up user

 * Preconditions: none
 * Postconditions: user created on server-side, no cookie generated yet
 * Parameters: a string representing the username, and a string representing the password
 * Returns: an error if request malfunctions
 */
func (c *Client) SignUp(username string, password string) (err error) {
	var ret string
	// sends username and passwords as arguments to handler
	err = c.server.Call("signup", &ret, username, password)
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	fmt.Println("signup successful, please log in or sign up another user")
	return nil
}

/*
 * LogIn() - calls loginHandler in server to sign up user, if successful sets up cookie on
 *					client side

 * Preconditions: user has access to username and password
 * Postconditions: cookie is validated for session on server-side and sent back
 * Parameters: a string representing the username, and a string representing the password
 * Returns: an error if request malfunctions
 */
func (c *Client) LogIn(username string, password string) (err error) {
	var ret string
	// sends username and passwords as arguments to handler
	err = c.server.Call("login", &ret, username, password)
	if err != nil {
		return client.MakeFatalError(err)
	}
	if !strings.Contains(ret, "cookie:") {
		return fmt.Errorf(ret)
	}
	// gets cookie from return value, writes it to bin as cookie.txt file
	ret = strings.TrimPrefix(ret, "cookie:")
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	err2 := ioutil.WriteFile(dir + "/cookie.txt", []byte(ret), 0777)
	if err2 != nil {
		fmt.Print(err)
	}
	fmt.Println("logged in")
	return nil
}

/*
 * Upload() - calls uploadHandler in server to upload file
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to upload, and byte array contining the file
 * Returns: an error if request malfunctions
 */
func (c *Client) Upload(path string, body []byte) (err error) {
	var ret string
	// sends cookie, path, and byte array body as arguments to handler
	err = c.server.Call("upload", &ret, getCookie(), path, body)
	// rest of code given by TA's
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	return nil
}

/*
 * Download() - calls downloadHandler in server to download file
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to download to
 * Returns: byte array of data if successful, and an error if request malfunctions
 */
func (c *Client) Download(path string) (body []byte, err error) {
	var ret internal.DownloadReturn
	// sends cookie, path as arguments to handler
	err = c.server.Call("download", &ret, getCookie(), path)
	// rest of code given by TA's
	if err != nil {
		return nil, client.MakeFatalError(err)
	}
	if ret.Err != "" {
		return nil, fmt.Errorf(ret.Err)
	}
	return ret.Body, nil
}

/*
 * List() - calls listHandler in server to list directory contents
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to list
 * Returns: an array directory entries if successful, and an error if request malfunctions
 */
func (c *Client) List(path string) (entries []client.DirEnt, err error) {
	var ret internal.ListReturn
	// sends cookie, path as arguments to handler
	err = c.server.Call("list", &ret, getCookie(), path)
	// rest of code given by TA's
	if err != nil {
		return nil, client.MakeFatalError(err)
	}
	if ret.Err != "" {
		return nil, fmt.Errorf(ret.Err)
	}
	var ents []client.DirEnt
	for _, e := range ret.Entries {
		ents = append(ents, e)
	}
	return ents, nil
}

/*
 * Mkdir() - calls mkdirHandler in server to make a directory in a given path
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to make directory
 * Returns: an error if request malfunctions
 */
func (c *Client) Mkdir(path string) (err error) {
	var ret string
	// sends cookie, path as arguments to handler
	err = c.server.Call("mkdir", &ret, getCookie(), path)
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	return nil
}

/*
 * Remove() - calls removeHandler in server to remove directory / file in a given path
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to remove dir / file
 * Returns: an error if request malfunctions
 */
func (c *Client) Remove(path string) (err error) {
	var ret string
	// sends cookie, path as arguments to handler
	err = c.server.Call("remove", &ret, getCookie(), path)
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	return nil
}

/*
 * PWD() - calls pwdHandler in server to get current working directory
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: none
 * Returns: a string represnting the current working directory, and an error if
 * 			request malfunctions
 */
func (c *Client) PWD() (path string, err error) {
	var ret internal.PWDReturn
	// sends cookie as arguments to handler
	err = c.server.Call("pwd", &ret, getCookie())
	if err != nil {
		return "", client.MakeFatalError(err)
	}
	if ret.Err != "" {
		return "", fmt.Errorf(ret.Err)
	}
	return ret.Path, nil
}

/*
 * CD() - calls cdHandler in server to change working directory
 *
 * Preconditions: user calling has cookie to be validated by server
 * Postconditions: none
 * Parameters: a string representing the path to change to
 * Returns: an error if request malfunctions
 */
func (c *Client) CD(path string) (err error) {
	var ret string
	// sends cookie, path as arguments to handler
	err = c.server.Call("cd", &ret, getCookie(), path)
	if err != nil {
		return client.MakeFatalError(err)
	}
	if ret != "" {
		return fmt.Errorf(ret)
	}
	return nil
}

// not implemented, we are in cs 166
func (c *Client) Share(path, username string, write bool) (err error) {
	return client.ErrNotImplemented
}

// not implemented, we are in cs 166
func (c *Client) RemoveShare(path, username string) (err error) {
	return client.ErrNotImplemented
}

// not implemented, we are in cs 166
func (c *Client) GetShares(path string) (shares []client.Share, err error) {
	return nil, client.ErrNotImplemented
}
