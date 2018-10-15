import requests, os, json
from threading import Thread
from time import sleep, time
import random, os


shard = random.randint(1, 5)
os.system("./dexmd mw wal.json " + str(shard))

wallet = json.loads(open('wal.json').read())
address = wallet["Address"]

validator = requests.get("http://35.211.241.218:5000/start_validator").text
validator = int(validator)

timestamp = requests.get("http://35.211.241.218:5000/submit_addr", params={
    "wallet": address
}).text
timestamp = int(timestamp)

if validator != 0:
    sleep(40)
    os.system("sudo ./dexmd sn wallet" + str(validator) + " " + str(timestamp))
else:
    sleep(60)

def send_dexmpos():
    print("waiting")
    sleep(timestamp-time()+20)
    req = requests.get("http://35.211.241.218:5000/send_money", params={
        "wallet": address
    })
    # maybe i should wait more than 10 sec, should be around max 200 sec
    sleep(200)
    if req == "Sent":
        print("SENDING TO DEXMPOS")
        os.system("./dexmd mkt wal.json DexmPoS 20 2")
    else:
        print("failed")


thread = Thread(target=send_dexmpos)
thread.start()


os.system("./dexmd sn wal.json " + str(timestamp))

thread.join()
