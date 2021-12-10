package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"strconv"

	"../internal"
	"../lib/support/rpc"

	"database/sql"
	"encoding/binary"
 	_"github.com/mattn/go-sqlite3"

 	"crypto/sha256"
 	"time"

 	"math/rand"
 	"encoding/hex"

 	"strings"
 	"path"
 	"path/filepath"

	"bytes"
)

// global variables:

const MAX_DB_STORAGE = 100000000 // in bytes, (100MB total db storage in system)
const MAX_USER_STORAGE = 5000000 // (in bytes, (5MB storage per user)
var db * sql.DB // our sql database
var abs_base_dir string //directory up to /bin on server (does not change)

func main() {

	// initialize dropbox sql database

	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	abs_base_dir = dir + "/"

	db, _ = sql.Open("sqlite3", abs_base_dir + "dropbox.db")

	// create sessions table
	statement, _ := db.Prepare("CREATE TABLE IF NOT EXISTS sessions (session_id TEXT PRIMARY KEY, username TEXT, expiration_date INTEGER)")
	statement.Exec()

	// create username-password table
	statement, _ = db.Prepare("CREATE TABLE IF NOT EXISTS u_p (username TEXT PRIMARY KEY, salt TEXT, hashword TEXT)")
	statement.Exec()

	// create metadata table
	statement, _ = db.Prepare("CREATE TABLE IF NOT EXISTS metadata (username TEXT PRIMARY KEY, root TEXT)")
	statement.Exec()

	// create username-present working directory table
	statement, _ = db.Prepare("CREATE TABLE IF NOT EXISTS user_pwd (username TEXT PRIMARY KEY, pwd TEXT)")
	statement.Exec()

	var listenAddr string

	// if "--reset" option is called, resets database, otherwise set up base directory and listener

	switch {
	case len(os.Args) == 2 && os.Args[1] == "--reset":
		resetdatabase()
		return
	case len(os.Args) == 3 && (len(os.Args[1]) == 0 || os.Args[1][0] != '-'):
		listenAddr = os.Args[2]
	default:
		fmt.Fprintf(os.Stderr, "Usage: %v [--reset | <base-dir> <listen-address>]\n", os.Args[0])
		os.Exit(1)
	}

	// declare rpc handlers for client-side calling functionality

	// new rpc handlers from our implementaion
	rpc.RegisterHandler("authenticate", authenticateHandler)
	rpc.RegisterHandler("signup", signupHandler)
	rpc.RegisterHandler("login", loginHandler)
	rpc.RegisterHandler("logout", logoutHandler)
	rpc.RegisterHandler("delete", deleteHandler)

	// rpc handlers given in the stencil code
	rpc.RegisterHandler("upload", uploadHandler)
	rpc.RegisterHandler("download", downloadHandler)
	rpc.RegisterHandler("list", listHandler)
	rpc.RegisterHandler("mkdir", mkdirHandler)
	rpc.RegisterHandler("remove", removeHandler)
	rpc.RegisterHandler("pwd", pwdHandler)
	rpc.RegisterHandler("cd", cdHandler)
	rpc.RegisterFinalizer(finalizer)

	// runs server
	err := rpc.RunServer(listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not run server: %v\n", err)
		os.Exit(1)
	}
}

/*
 * checkNestedPath() - checks how much nesting is in a path, for example root/dir1
 *										 would have a nesting of 1, while root/dir1/dir2 would have
 *										 a nesting of 2, and there can be at most 20
 * Parameters:
 *	- path: a string representing the path to the new directory
 *	- root: a string representing the root of the user
 *
 * Returns:
 *	- a boolean representing if the nested path requirements are passed
 */
func checkNestedPath(path string, root string) bool {
	path_arr := strings.Split(path, root)
	path = path_arr[1]
	nested_num := strings.Count(path, "/")
	if (nested_num > 20) {
		return false
	}
	return true
}

/*
 * checkSizeName() - checks if name of file/folder is less than 25 characters to
 *									 prevent overflow, also checks that the size of the user's
 *									 root directory plus the uploaded file / new directory made
 *									 is less than the user storage limit
 *
 * Parameters:
 *	- cookie: a string representing user's cookie
 *  - add_size: an int representing the number of bytes in the upload, or -1 if
 * 							if is a directory
 *  - path: a string representing path to the new file / directory
 *
 * Returns:
 *	- a boolean representing if the name and size requirements are passed
 */
func checkSizeName(cookie string, add_size int, path string) string {
  // get last element in path (the name)
	path_array := strings.Split(path, "/")
	len_path_array := len(path_array)
	name := path_array[len_path_array - 1]

	// check that the name is not greater than 25 characters
	if (len(name) > 25) {
		return "name cannot be greater than 25 characters"
	}

	// if it is a folder, set byte size to that of empty folder
	if (add_size == -1){
		add_size = 5000
	}

	// build full path to root
	_, username := authenticateRequest(cookie)
	_, root := rootForUsername(username)
	full_root_path := abs_base_dir + root

	// use full path to root to execute du (disk usage) linux command, "-sk" give size in kilobytes
	cmd := exec.Command("du", "-sk", full_root_path)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "issue getting size"
	}

	// get just number from "du" command output, convert to bytes from kb
	size_array := strings.Split(out.String(), "\t")
	cur_byte_size, err := strconv.Atoi(size_array[0])
	if err != nil {
		return "issue getting size"
	}
	cur_byte_size = cur_byte_size * 1000

	// if new size is less than max user storage, allow upload or mkdir call
	new_size := add_size + cur_byte_size
	if (new_size > MAX_USER_STORAGE){
		return "user storage exceeded, cannot perform this task"
	}

	return ""
}

/*
 * resetdatabase() - removes all root directories, then sql database itself
 *
 * Parameters: none
 *
 * Returns:
 *	- an string with an error message, or empty upon success
 */
func resetdatabase() string {
	// Query all users, remove each root
	rows, _ := db.Query("SELECT username, root FROM metadata")
	var username string
	var root string
	for rows.Next() {
			rows.Scan(&username, &root)
			err := os.RemoveAll(root)
			if err != nil {
				return "error removing root folers in reset"
			}
	}
	rows.Close()

	// remove dropbox sql database
	err := os.Remove("dropbox.db")
	if err != nil {
		return "error removing databse in reset"
	}
	return ""
}

/*
 * resetdatabase() - deletes session with given username if it exists
 *
 * Parameters: username: a string representing the username
 * Returns: nothing
 */
func deleteSessionWithUsername(username string) {
	statement, _ := db.Prepare("DELETE FROM sessions WHERE username = ?")
	statement.Exec(username)
}

/*
 * resetdatabase() - deletes session with given cookie if it exists
 *
 * Parameters: cookie: a string representing the user's session id
 * Returns: a string with an error message, or empty upon success
 */
func logoutHandler(cookie string) string {
	// hash cookie so that it matches up in db
	h := sha256.New()
	h.Write([]byte(cookie))
	sha256_hash_cookie := hex.EncodeToString(h.Sum(nil))
	// delete hashed cookie
	statement, _ := db.Prepare("DELETE FROM sessions WHERE session_id = ?")
	_, err2 := statement.Exec(sha256_hash_cookie)
	if err2 != nil {
		return "error logging out"
	} else {
		return "" // success
	}
}

/*
 * deleteHandler() - delete all of the user information and their data (API command)
 *
 * Parameters: cookie: a string representing the user
 * Returns: a string with an error message, or empty upon success
 */
func deleteHandler(cookie string) string {

	err_message := "could not delete account"

	// authenticate
	err0, username := authenticateRequest(cookie)
	if (err0 != "") {
		return err0
	}

	// get root
	err1, root := rootForUsername(username)
	if (err1 != "") {
		return err1
	}

	// delete all table information for that username (i.e. in session, u_p, metadata, and user_pwd)
	statement, _ := db.Prepare("DELETE FROM sessions WHERE username = ?")
	_, err := statement.Exec(username)
	if err != nil {
		return err_message
	}

	statement, _  = db.Prepare("DELETE FROM u_p WHERE username = ?")
	_, err = statement.Exec(username)
	if err != nil {
		return err_message
	}

	statement, _  = db.Prepare("DELETE FROM metadata WHERE username = ?")
	_, err = statement.Exec(username)
	if err != nil {
		return err_message
	}

	statement, _  = db.Prepare("DELETE FROM user_pwd WHERE username = ?")
	_, err = statement.Exec(username)
	if err != nil {
		return err_message
	}

	//remove
	err = os.RemoveAll(abs_base_dir + root)
	if err != nil {
		return err_message
	}

	return ""
}

/*
 * authenticateHandler() - authenticates user by checkign if cookie exists in
 * 												 the session database
 *
 * Parameters: cookie: a string representing the user's cookie
 * Returns: a string with an error message, or empty upon success
 */
func authenticateHandler(cookie string) string {


	//hash the cookie to validate against the value in the database
	h := sha256.New()
	h.Write([]byte(cookie))
	sha256_hash_cookie := hex.EncodeToString(h.Sum(nil))

	// query for entry with cookie
	statement, _ := db.Prepare("SELECT * FROM sessions WHERE session_id = ?")
	rows, err1 := statement.Query(sha256_hash_cookie)
	if err1 != nil {
		return err1.Error()
	}
	var session_id string
	var username string
	var expiration_date int64
	// if entry exists and cookie not expired
	if rows.Next() {
		rows.Scan(&session_id, &username, &expiration_date)
		rows.Close()
		if (expiration_date > time.Now().UTC().UnixNano()){

			// get root from username and move to the root for user
			err2, root := rootForUsername(username)
			if err2 != "" {
				return err2
			}
			err := os.Chdir(abs_base_dir + root)
			if err != nil {
				return "authentication failed"
			}

			// Reset the pwd upon login / authenticate
			pwd := abs_base_dir + root
			statement, _ := db.Prepare("update user_pwd set pwd = ? where username = ?")
			statement.Exec(pwd, username)

			return username
		} else {

			// Delete all sessions for that user because its expired
			deleteSessionWithUsername(username)

			return "false, session expired"
		}
	} else {
		return "false, invalid cookie"
	}
}

/*
 * getUserPWD() - get pwd from username
 *
 * Parameters: username: a string representing the user's username
 * Returns: a string with an error message, or empty upon success
 */
func getUserPWD(username string) string {
	statement, _ := db.Prepare("SELECT * FROM user_pwd WHERE username = ?")
	rows2, err := statement.Query(username)
	if err != nil {
		return err.Error()
	}

	//grab username and salt, as well as the hashed password
	var pwd string
	if rows2.Next() {
		rows2.Scan(&username, &pwd)
	} else {
		return "Username does not exist. Error retrieving pwd."
	}
	rows2.Close()

	return pwd
}

/*
 * signupHandler() - signs up user if space available in database
 *
 * Parameters:
 * 		- username: a string representing the user's username
 * 		- password: a string representing the user's password
 * Returns: a string with an error message, or empty upon success
 */
func signupHandler(username string, password string) string {
	// query all users and sum up size of all root directories
	rows0, _ := db.Query("SELECT username, root FROM metadata")

	total_root_byte_sum := 0

	var username0 string
	var user_root string
	for rows0.Next() {
		rows0.Scan(&username0, &user_root)

		// build root path and find size from disk usage (du) command
		full_root_path := abs_base_dir + user_root
		cmd := exec.Command("du", "-sk", full_root_path)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			return "error in signupHandler!!!"
		}
		// split on tab because we just want number part of du output
		size_array := strings.Split(out.String(), "\t")
		cur_byte_size, err := strconv.Atoi(size_array[0])
		if err != nil {
			return "error in signupHandler!!!"
		}

		// get byte size and add to total, plus the size of an empty folder
		cur_byte_size = cur_byte_size * 1000
		total_root_byte_sum = total_root_byte_sum + cur_byte_size + 4096
	}

	rows0.Close()


	if (total_root_byte_sum > MAX_DB_STORAGE){
		return "Database full, cannot sign up new users"
	}

	// now check if username already exists before signup process
	statement, _ := db.Prepare("SELECT * FROM u_p WHERE username = ?")
	rows, err1 := statement.Query(username)
	if err1 != nil {
		return err1.Error()
	}
	if rows.Next() {
		rows.Close()
		return "This username already exists. Please sign up with a different username."
	}
	rows.Close()

	// check password meets password requirments
	if !strings.ContainsAny(password, "0123456789") {
		return "password must contain numbers"
	}
	lowercase := strings.ContainsAny(password, "abcdefghijklmnopqrstuvwxyz")
	uppercase := strings.ContainsAny(password, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if !(lowercase && uppercase) {
		return "password must contain upper and lowercase letters"
	}

	// generate salt
	var salt_string string = generateRandomHexString()

	// generate new hash function and hash salted password
	h := sha256.New()
	h.Write([]byte(password + salt_string))
	sha256_hash := hex.EncodeToString(h.Sum(nil))

	// insert username with salt and hashed-password into sql table
	statement, _ = db.Prepare("INSERT INTO u_p (username, salt, hashword) VALUES (?, ?, ?)")
	_, err2 := statement.Exec(username, salt_string, sha256_hash)
	if err2 != nil {
		return "Error Signing Up"
	}

	// generate root for the user
	root := "r" + generateRandomHexString()

	//add new user informaiton to metadata table
	statement, _ = db.Prepare("INSERT INTO metadata (username, root) VALUES (?, ?)")
	_, err2 = statement.Exec(username, root)
	if err2 != nil {
		return "Error Signing Up"
	}

	//create directory for this user at root
	err := os.Mkdir(abs_base_dir + root, 0775)
	if err != nil {
		return "could not make root directory"
	}

	// initialize present working directory and put in table
	pwd := abs_base_dir + root
	statement, _ = db.Prepare("INSERT INTO user_pwd (username, pwd) VALUES (?, ?)")
	_, err2 = statement.Exec(username, pwd)
	if err2 != nil {
		return "Error Signing Up"
	}

	return ""
}

/*
 * generateRandomHexString() - generates random hex string with pseudo random number
 * 				with 2 sources of randomness, used for salt, cookie, root directory name
 *
 * Parameters: none
 * Returns: random hex string
 */
func generateRandomHexString() string {
	// first layer of randomness for seed, a random stream of 8 bytes(64 bits)
	firstlayer := make([]byte, 8)
  rand.Read(firstlayer)
  var firstlayer_int64 int64 = int64(binary.LittleEndian.Uint64(firstlayer))

	//second layer of randomness for seed, current millis epoch time
  var secondlayer_int64 int64 = time.Now().UTC().UnixNano()

  //set seed and produce random string
	rand.Seed(firstlayer_int64 + secondlayer_int64)
	var positive_max_value int64 = 9223372036854775807 // largest positive value of int64

	// generate random int and convert into hex
	var random int64 = rand.Int63n(positive_max_value)
	random_string := strconv.FormatInt(random, 16)

	return random_string
}

/*
 * loginHandler() - logs in user with correct username and password
 *
 * Parameters:
 * 		- username: a string representing the user's username
 * 		- password: a string representing the user's password
 * Returns: a string with the cookie to be stored in the client
 */
func loginHandler(username string, password string) string {
	statement, _ := db.Prepare("SELECT * FROM u_p WHERE username = ?")
	rows2, err := statement.Query(username)

	if err != nil {
		return err.Error()
	}

	// grab username and salt, as well as the hashed password
	var salt_string string
	var hashword string
	if rows2.Next() {
		rows2.Scan(&username, &salt_string, &hashword)
	} else {
		return "Username does not exist. Please try again."
	}
	rows2.Close()

	// re-hash password with salt and see if it matches hashed password in database
	h := sha256.New()
	h.Write([]byte(password + salt_string))
	sha256_hash := hex.EncodeToString(h.Sum(nil))

	if (0 == strings.Compare(sha256_hash, hashword)){
			var return_cookie string = generateRandomHexString()

			//hash the cookie to store in actual db, return the non hashed to user
			h := sha256.New()
			h.Write([]byte(return_cookie))
			sha256_hash_cookie := hex.EncodeToString(h.Sum(nil))

			//before adding new user, delete any old sessions for that user if they exist
			deleteSessionWithUsername(username)

			// add new session
			statement, _ = db.Prepare("INSERT INTO sessions (session_id, username, expiration_date) VALUES (?, ?, ?)")
			exp_date := time.Now().UTC().UnixNano() + 600000000000
			_, err2 := 	statement.Exec(sha256_hash_cookie, username, exp_date)
			if err2 != nil {
				return "could not provide session"
			}

			// RESET the pwd upon login / authenticate
			_, root := rootForUsername(username)
			pwd := abs_base_dir + root
			statement, _ := db.Prepare("update user_pwd set pwd = ? where username = ?")
			statement.Exec(pwd, username)

			// start in pwd
			err = os.Chdir(pwd)
			if err != nil {
				return "directory not found"
			}

			return "cookie:" + return_cookie
	} else {
			return "Username/Password Incorrect"
	}
}

/*
 * authenticateRequest() - take in cookie, make sure it exists in the database so
 * 						the request is valid because it is impossible to forge cookies
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * Returns: a string tuple, with the error in the first part and the username in the
 * 				second if the request is valid
 */
func authenticateRequest(cookie string) (string, string) {
	// query for session with given cookie
	statement, _ := db.Prepare("SELECT username FROM sessions WHERE session_id = ?")

	//hash the cookie to validate against the value in the database
	h := sha256.New()
	h.Write([]byte(cookie))
	sha256_hash_cookie := hex.EncodeToString(h.Sum(nil))

	rows, err1 := statement.Query(sha256_hash_cookie)
	if err1 != nil {
		return "could not authenticate request", ""
	}

	// if exists then return corresponding username
	var username string
	if rows.Next() {
		rows.Scan(&username)
	} else {
		return "could not authenticate request", ""
	}
	rows.Close()

	return "", username
}

/*
 * authenticateRequest() - get root for a given username, first string is error message,
 *              second is root if no error
 *
 * Parameters:
 * 		- username: a string representing a valid username
 * Returns: a string tuple, with the error in the first part and the root in the
 * 				second if the request is valid
 */
func rootForUsername(username string) (string, string) {
	// query for entry with given username
	statement, _ := db.Prepare("SELECT root FROM metadata WHERE username = ?")
	rows, err1 := statement.Query(username)
	if err1 != nil {
		return "could not find root", ""
	}

	// if exists then return corresponding root
	var root string
	if rows.Next() {
		rows.Scan(&root)
	} else {
		return "could not find root", ""
	}
	rows.Close()

	return "", root

}

/*
 * validatePath() - makes sure that path specified with pwd is valid for the given root,
 *              including edge cases (i.e. ../../etc) so user does not end up outside root,
 *
 * Parameters:
 * 		- pwd: a string representing a valid username
 * 		- p: a string representing the user-inputted path
 * 		- root: a string representing the user's root
 *
 * Returns: a string tuple, with the error in the first part and a path in the
 * 				second if the path is valid
 */
func validatePath(pwd string, p string, root string) (string, string) {

	// clean path to account for ".." and see if path already is valid absolute without pwd
	result := path.Clean(p)

	// if absolue path
	if strings.HasPrefix(result, "/") {
		return "", abs_base_dir + root + result
	}

	// joing the pwd and path
	result = path.Clean(path.Join(pwd, p))

	// if beyond root, return
	if strings.HasPrefix(result, abs_base_dir + root) {
		return "", result
	} else {
		return "", abs_base_dir + root
	}
}

/*
 * performChecks() - calls authenticateRequest() and validatePath() to validate
 * 					user and path selected
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted path
 *
 * Returns: a string tuple, with the error in the first part and a path in the
 * 				second if the path is valid
 */
func performChecks(cookie string, path string) (string, string) {

	// authenticate
	err, username := authenticateRequest(cookie)
	if (err != "") {
		return err, ""
	}

	// get root
	err, root := rootForUsername(username)
	if (err != "") {
		return err, ""
	}

	// validate path with the root
	err, path = validatePath(getUserPWD(username), path, root)
	if (err != "") {
		return err, ""
	}

	return "", path
}

/*
 * uploadHandler() - uploads file to a given location,
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted path
 * 		- body: a byte array representing the data
 *
 * Returns: a string with an error message, or empty upon success
 */
func uploadHandler(cookie string, path string, body []byte) string {
	// perform checks to validate user and action
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return err0
	}

	// check upload size is valid
	str := checkSizeName(cookie, len(body), path)
	if (str != "") {
		return str
	}

	// use linux commands to write file (code given to us by TAs)
	err := ioutil.WriteFile(path, body, 0664)
	if err != nil {
		return "could not write file"
	}
	return ""
}

/*
 * downloadHandler() - downloads file to a given location
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted path to upload to
 *
 * Returns: an internal.DownloadReturn with error or body on success
 */
func downloadHandler(cookie string, path string) internal.DownloadReturn {
	// perform checks to validate user and action
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return internal.DownloadReturn{Err: err0}
	}

	// use linux commands to download contents (code given to us by TAs)
	body, err := ioutil.ReadFile(path)
	if err != nil {
		return internal.DownloadReturn{Err: "coud not read specified file"}
	}
	return internal.DownloadReturn{Body: body}
}

/*
 * listHandler() - lists file to a given location
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted directory path to list
 *
 * Returns: an internal.ListReturn with error or DirEnt on success
 */
func listHandler(cookie string, path string) internal.ListReturn {
	// perform checks to validate user and action
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return internal.ListReturn{Err: err0}
	}

	// use linux commands to list contents (code given to us by TAs)
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		fmt.Println(err.Error())
		return internal.ListReturn{Err: "could not read specified path"}
	}
	var entries []internal.DirEnt
	for _, fi := range fis {
		entries = append(entries, internal.DirEnt{
			IsDir_: fi.IsDir(),
			Name_:  fi.Name(),
		})
	}
	return internal.ListReturn{Entries: entries}
}

/*
 * mkdirHandler() - makes directory at a given location
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted directory path to list
 *
 * Returns: a string with an error message, or empty upon success
 */
func mkdirHandler(cookie string, path string) string {
	// perform checks to validate user and action
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return err0
	}

	// make sure enough user storage space left to create directory
	str := checkSizeName(cookie, -1, path)
	if (str != "") {
		return str
	}

	// get root
	_, username := authenticateRequest(cookie)
	_, root := rootForUsername(username)

	// make sure directory nesting does not exceed nesting limits
	if !checkNestedPath(path, root){
		return "Too many nested files in path, must be less than 20"
	}

	// make sure directory addition does not exceed 20 sub-directory limit in one directory
	num_directories := 0

	// get array of FileInfo's from ReadDir()
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return "Error in ReadDir()"
	}

	// for all files in the directory, if it's a directory (not a file) increment num_directories
	for _, file := range files {
		if file.IsDir() {
			num_directories++
		}
	}

	// make sure directory addition does not exceed 20 sub-directory limit in one directory
	if num_directories > 19 {
		return "Too many sub-directories in this directory"
	}
	err = os.Mkdir(path, 0775)
	if err != nil {
		return "could not make path at specified path"
	}
	return ""
}

/*
 * removeHandler() - removes file or directory at a given location
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted directory path to list
 *
 * Returns: a string with an error message, or empty upon success
 */
func removeHandler(cookie string, path string) string {
	// perform checks to validate user and action
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return err0
	}

	// make sure we are not deleting the root
	_, username := authenticateRequest(cookie)
	_, root := rootForUsername(username)

	if path == abs_base_dir + root {
		return "cannot remove root directory"
	}

	// use linux commands to remove file/directory (code given to us by TAs)
	err := os.Remove(path)
	if err != nil {
		return "could not remove at specified path"
	}
	return ""
}

/*
 * pwdHandler() - list current working directory
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 *
 * Returns: an internal.PWDReturn with error or path on success
 */
func pwdHandler(cookie string) internal.PWDReturn {
	// check that request comes from valid user
	err0, username := authenticateRequest(cookie)
	if (err0 != "") {
		return internal.PWDReturn{Err: err0}
	}

	// get pwd from user_pwd table
	path := getUserPWD(username)

	// get root from username
	err1, root := rootForUsername(username)
	if (err1 != "") {
		return internal.PWDReturn{Err: err1}
	}

	// trim pwd path so only user sees stuff past root directory
	bad_path := abs_base_dir + root
	path = strings.TrimPrefix(path, bad_path)

	if path == "" {
		path = "/"
	}

	return internal.PWDReturn{Path: path}
}

/*
 * cdHandler() - move into path
 *
 * Parameters:
 * 		- cookie: a string representing the user's cookie
 * 		- path: a string representing the user-inputted directory path to list
 *
 * Returns: an internal.PWDReturn with error or path on success
 */
func cdHandler(cookie string, path string) string {
	// check that request comes from valid user
	err0, path := performChecks(cookie, path)
	if err0 != "" {
		return err0
	}

	// valid path and request up to this point
	err := os.Chdir(path)
	if err != nil {
		return "could not change directory to specified path"
	}

	// update the pwd table
	_, username := authenticateRequest(cookie)
	statement, _ := db.Prepare("update user_pwd set pwd = ? where username = ?")
	statement.Exec(path, username)

	return ""
}

// given as part of TA code, called when we shut down ./server binary
func finalizer() {
	fmt.Println("Shutting down...")
}
