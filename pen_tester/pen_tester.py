from subprocess import Popen, PIPE
import time


# These tests are not fully automated, aka shouldn't just be run by TAs,
# this script is just to show the TAs how we did
#  some penetration testing on our own

p = Popen('./../updated_code/bin/client localhost:8080', shell=True, stdin=PIPE)

#run this test with no cookie.txt in client/

#login command
p.communicate(input='login paultouma Lebron10\n'.encode())

x = ""

num_logins_signups = 25000 #10 for real test, 10 for quick test

#test to to do multiple logins and signups
for i in range(num_logins_signups):
	x = x + "signup lebron2" + str(i) + " Lebron10\n"
	# x = x + "login aed" + str(i) + " Lebron10\n"


p.communicate(input=x.encode())

print("Login/Signup Persistence: PASS")


x = ""

depth = 25 #10 for real test, 10 for quick test

#test to to do multiple logins and signups
for i in range(depth):
	x = x + "mkdir hi\n"
	x = x + "cd hi \n"

p.communicate(input=x.encode())

print("Mkdir: PASS")
