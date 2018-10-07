from flask import Flask, request
from time import time
import os, random
app = Flask(__name__)

wallets = [] 
start_time = int(time() + 120)

@app.route("/submit_addr")
def key():
    wallet = request.args.get('wallet')
    wallets.append(wallet)

    return str(start_time)


@app.route("/send_money")
def send_money():
    if random.random() > 0.5:
        wallet = request.args.get('wallet')
        os.system("./dexmd mkt w3 " + wallet + " 100 2")
        return "Sent"
    else:
        return "Not Sent"

app.run(host='0.0.0.0')