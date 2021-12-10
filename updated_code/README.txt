README

--------------------------------------------------------------------------------------------------

CSCI 1660 - Computer Systems Security

Project 4 - Dropbox

Second Handin: Implementation

--------------------------------------------------------------------------------------------------

PART 1: Changes made to our original design during implementation

from Authentication:

- Session ids (i.e. cookies) are not just stored as is in the database. We hash the session id's
before storing them (and also hash after sent from the client when authenticating requests) in
order to ensure that if the sessions/username/expiration_date table were stolen, it would not
immediately allow an attacker to impersonate a legitimate user. In the original design we simply
just stored session id's in a SQL table.

- We set the expiration date to 10 minutes, not 1 day in the original design.

from Access Control:

- Access control is the same as described in the DESIGN.pdf. We send a cookie with each request
to authenticate / identify the user calling the request, and make sure their calling path matches
with their own root to make sure they can only preform actions within their root directory.

- Also we keep track of the PWD in a username-pwd table on the server side, starting them out in
the root after each login and updating their PWD at each call to "cd" in the client. We use the
pwd to build their path when only a relative path is given.

from File Storage:

- Instead of using SQL tables for each user to "mimic the filesystem" with a "File/Folder Name
Table" with fields such as: is it a file or folder, parent directory, list of sub-directories in
it (if a directory), etc, we actually make root directories and sub directories for each user.
We are essentially making a linux filesystem on the server end to mirror the filesystem seen on
the client side, and the os package in golang gives us the same linux-type commands (i.e. cd, mkdir,
ls) from the command line, making implementing API calls in the client really straightforward.
We made this design switch because it made implementing functionality much easier. Instead of writing
code to recursively traverse SQL tables with all the files and folders of a user as entries, we can
simply call ioutil.ReadDir(path) on a path to the actual folder, which does all of this work for us.

- In the same vein, instead of having a "File Content SQL Table" where we store "blobs" of data in
actual SQL, we simply store files in the actual directories or sub-directories we created in the
linux-style filesystem.

- Thus, given a path supplied by the client, we do not "iteratively search through the File/Folder
Name Table" since this does not exist. Instead, determining whether the path identifies a file,
folder, or doesn't exist is conveniently handled by the os commands themselves, handling errors
appropriately.

from API:

- API is the same as described in DESIGN.pdf from an authentication and functionality standpoint.
The only difference is (going along with the client.go implementation given to us) we often used
string return values to both indicate errors (if the string was not empty) and also contain the error
as the body of the string.

from Client Computation

- Client Computation is the same as described in DESIGN.pdf, basically no important computation is
done on the client side.

mitigating risk of vulnerabilities and verification are explained in the next two parts:

--------------------------------------------------------------------------------------------------

PART 2: All testing / verification done to verify that service is secure

There were two high level components of our dropbox: functionality and security.

From a functionality standpoint, we had to make sure our dropbox worked. There were three aspects
of this:

- First, we did system tests by simply using our Dropbox itself: signing up and logging in as a new
user, and going through all the API commands (mkdir, cd, ls, upload, etc.) - with edge cases such
as putting multiple slashes at the end of the path - to make sure that the dropbox worked as specified
in the handout.

- Second, we opened up two clients and alternated API requests to make sure that the database could
support multiple clients at the same time. IMPORTANTLY, after logging into each account, you have to
save the cookie written to cookie.txt in /bin (maybe in an open text editor). This is because the
cookie.txt file in /bin will be overwritten when the second user logs in and the sever will think all
requests are coming from the same user. So, to test the functionality and "mimic" two clients with
two open client windows from the same machine you have to MANUALLY switch the cookie.txt file when
alternating calls for each running client, and the dropbox works as expected.

Of course, this scenario would never actually happen in real life because two users will be on different
machines, so one client implementation would not be store multiple cookies from different users for the
same Web App at the same time. But just for testing multiple-client functionality this is how you can do
this.

- Third, we ran the testing suite given to us by the TA's in test.go, and all tests PASSED. It's a
bit complicated to do this, but here are the steps:

1) cd into /bin and reset the server with ./server --reset
2) in client.go, on line 92, set dir to your own pwd from bin, and comment-out the line with our pwd
3) save and make everything
4) run the server in /bin
5) run client in /bin, signup and login to the client so that there is an authenticated cookie
   on the client side
6) exit the client with Control-C
7) cd into /client and run "go test" on the command line
8a) to re-run the test, reset the server and then start with Step 4 and repeat
8b) to perform other functionality / security checks after running the test, also reset the server and
    then do whatever

From a security standpoint, there are multiple potential vulnerabilities we focused on and tested.
An example of some the ~automated penetration testing that we did can be found in the "/pen_tester"
folder outside of the "updated_code" directory. Note: These tests are not intended to be run by
the TAs, but rather are to show TAs some of the automated tests we created for penetration testing.
Below are vulnerabilities we focused on when testing:

1) A user going out of their root:

One of the main exploits for a linux filesystem design such as ours was that the user could go out
of the root directory to read, access, and modify files from other users. In order to prevent this,
we preformed significant path sanitisation and parsing on the path argument of the API methods on
the server side. More specifically in validatePath() on the server, we use path.Clean(), path.Join(),
and strings.HasPrefix() in combination with knowledge of the user's pwd and root (which we derive
from the cookie they necessarily send to the server to authenticate each API) to make sure that any
path supplied to the os commands in our server-implemented API stays within the user's own root.
Otherwise, an error is thrown or nothing happens, depending on if the request is simply invalid or
if it is valid but is malicious.

Another important part of this is that the name of the root directory is hidden from the user in the
client terminal and is created from a randomly generated hex string. In this way, the user has no
knowledge of their root name (or anyone else's besides computationally unfeasible random guessing).
So even if they could somehow go cd into another directory (which they cannot do because of the above),
they wouldn't know the actual name of it to use in the path itself.

In order to test this, we made multiple users and with multiple roots, and simply tried our best to
cd out of our root with different malicious paths such including various combinations of ".." and
slashes, as well as absolute paths to other directories such as /client or /lib, and other roots.

None of these worked, and we were never able to leave our own root directory, so we are confident
that the user cannot go out of their root and exploit this vulnerability.

2) Denial of service attacks

Another major aspect is denial of service, where a malicious attacker can overload the database
with infinite signups, folders, or upload data to basically break the server and "deny service"
to anyone else.

In order to prevent this, we imposed a cap on the per user storage and total database storage
capacity, and before allowing commands such as signup, upload, or mkdir to execute, we check the
size of the database or root directory as needed to make sure the space required by the action
preformed will not exceed the limit imposed by our total database and user storage caps.

In order to test this, we did multiple things. First, to check that one user could not go above
their storage limit and max out the database, we attempted to uploaded files that were above
this cap, as well as multiple files hat added up to being too big as a logged-in user.
Neither of these attacks worked.

Second, we checked our total database storage limit worked by signing up many users and using all
the storage space in each until the aggregate data usage of all the users went over the storage cap.
As expected, the server caught this and did not allow any more users to sign up after this point, so
in this way too many normal users could overflow the total database limit. And that attack would
not work.

Third, to check that an one person could not signup up infinite users but break the database by
uploading, we counted an empty root directory as 4096 bytes (the storage allocated for an empty folder)
root and aggregated that when counting root sizes. So, after writing a script that signed up
infinite users with empty directories, which could feasibly break things server, this was still
caught by the server and the attack did not work.

Finally, we checked the one user could not upload infinite empty folders and break the database. We
did no impose a folder limit but instead imposed a path limit on the number of nested paths (20),
as well as a limit on the number of sub-directories allowed in any one directory (20). These are
limits are implied by our other restrictions on maximum user storage, not on directories themselves,
and when testing this we were unable to upload infinite directories due to these implied restrictions.
This this fourth denial of service attack did not work either.

3) Password protections against Weak passwords, Dictionary attacks, and Brute Force attacks

Another exploit that we prevented was password protection. Specifically, you don't want users to
be allowed to choose passwords such as "pass" as they are vulnerable to brute-force guessing attacks.
Thus, in the signupHandler() in our server, we parse the password and enforce number as well as upper-
lowercase requirements on it.

In order to test this, we tried passwords such as "1", "pass", "1pass", "PASS", "1PASS", and "Pass".
None of these worked, only "1Pass" - which has numbers, and upper and lowercase letters, allowed us to
sign up, confirming that this vulnerability did not work.

Even further, each password on our database is first salted, then hashed with a cryptographic hashing
function. Specifically, each salt is generated randomly with 64 bits of entropy, and is concatenated
with the password before hashed by SHA-256. The reason why salt (with relatively high entropy) is used
is to prevent from dictionary and brute force attacks. In other words, no malicious attacker can gain
any information about the stored hashes of other passwords by compromising one user’s password, as no
two users with the same passwords will have the same hash. Therefore, our implementation is resistant
to Dictionary attacks.

Finally, each hash of a salted password is 256 bits. Since hashing functions are one way, it would be
essentially impossible for someone to find the password that led to that hash. Therefore, our
implementation is resistant to brute force attacks.

4) Buffer overflow

Another exploit to prevent was buffer overflow attacks, more specifically that the name of a file or
folder uploaded / made by the client be so long that it itself crashes our sever. In order to prevent
this, we parse on the path supplied as an argument in the relevant function calls and if the last
argument (i.e. the name) has more than 25 characters (what we thought would be a reasonable limit)
we return an error.

To test this, we tried to make directories or upload files with names longer than this and it did not
work, so we are confident that this exploit has been avoided.

5) SQL Injection

Another possible exploit was SQL injections, which could happen because the user-inputted malicious
SQL code into the username or password fields when signing up or logging into our Dropbox, to do
things such as log in as other users or erase entire SQL tables (i.e. the DROP TABLE command).

In order to prevent this, we used prepared statements when we executed any SQL Queries involving
user-inputted fields in the server, with the statement.Prepare() and statement.Exec() golang functions.
Prepared statements are a a two-phase SQL command statement with ? placeholders, and values are
transmitted and compiled at a later phase than the parsing and compiling of the SQL query template itself.
And in this way, SQL injection cannot happen, because the user input simply acts as value
parameters.

In order to test this, we tried different SQL injection code exploits, such as adding "OR 1=1" in the
username and password fields in the login window. Moreover, while in the client itself, we used mixed in
similar SQL injection code into our API commands.

None of these worked (we could not log in as someone else, or execute DROP TABLE commands) so we are
confident that the user cannot preform a SQL injection.

6) Session hijacking

If the session cookie were guessable or constructed not randomly, a malicious user could hijack
another authenticated user’s session by predicting their session ID.

In order prevent this, we use a random number generator with two sources of randomness for the
seed, a random stream of 8 bytes (64 bits) as well as the current UTC time in nanoseconds, to
generate an integer (with maximum value being the largest possible int64), which we then
convert into hex as our cookie.

And we know from lecture that a random a 64 bit integer (given the two layers of randomness), should
have sufficient entropy to be computationally unfeasible to guess, so this attack would not work.

--------------------------------------------------------------------------------------------------

PART 3: Any vulnerabilities discovered during testing / verification, how we fixed them

There were a couple vulnerabilities we discovered during testing / verification that we then fixed:

1) Deleting the root

During our testing, we realised that if you type "rm ../" you can remove the root directory, since
technically the path to the root is a valid path argument (and is necessarily used in commands such
as "ls"). But of course, we don't want the user to be able to remove the root, so in order to fix
this we simply check in just the rmHandler() specifically, if the verified path just goes to the root,
then we do not let this command execute and throw an error.

2) Exposing metadata

Another vulnerability we found when using the API commands was when the "cd" or "mkdir" would through
errors, they would return the entire base directory path in the error, exposing hidden parts of the
filesystem as well as name of the user's root directory. As discussed previously, this is definitely
a vulnerability, and to fix it instead of returning a system error we returned custom error messages,
such as with cd, "could not change directory to specified path", instead.

3) Hashing session id's

We realised when we were almost done that for the same reasons why you salt and hash the password,
you want to hash the cookie because it is important authentication information that could be stolen
by a malicious attacker. By hashing the session id, simply taking the sessions table would still not
allow someone to become a user, because it is already hashed, and working the hash backward to the
cookie (which is what the server takes in in the authentication process to generate and check its
hash) is not possible. So hashing session id's was important and something we fixed.
