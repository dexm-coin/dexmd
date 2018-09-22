from flask import Flask, request
from time import time
app = Flask(__name__)

wallets = [] 
start_time = int(time() + 120)

@app.route("/submit_addr")
def key():
    wallet = request.args.get('wallet')
    wallets.append(wallet)

    return str(start_time)


app.run()