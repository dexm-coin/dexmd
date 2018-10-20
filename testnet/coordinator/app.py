from flask import Flask, request
import time
import os, random
from threading import Thread
app = Flask(__name__)

os.system("cd ..; cd ..; go build; cp dexmd testnet/coordinator/; cd testnet/coordinator/")

wallets = [] 
start_time = int(time.time() + 200)

def start_hackney():
    os.system("sudo ./dexmd sn wallet1 " + str(start_time) + " hackney true")

thread = Thread(target=start_hackney)
thread.start()

counter = 1

@app.route("/start_validator")
def start_validator():
    global counter
    if counter < 5:
        counter += 1
        return str(counter)
    return "0"

@app.route("/submit_addr")
def key():
    wallet = request.args.get('wallet')
    wallets.append(wallet)
    return str(start_time)

@app.route("/send_money")
def send_money():
    if random.random() > 0.5:
        print("REQUEST FOR DEXMPOS OK")
        wallet = request.args.get('wallet')
        os.system("./dexmd mkt wallet1 " + wallet + " 100 2")
        return "Sent"
    return "Not Sent"

app.run(host='0.0.0.0')
thread.join()
