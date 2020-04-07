# -*- coding: utf-8 -*-
import json
import base64
from subprocess import Popen, PIPE

def getPwd(req):
    #pwd = ""
    if req["HttpAction"] == "FunExecPwd":
        p = Popen(["bash", "-c", "cat /tmp/"+req["RequestId"]], stdout=PIPE, stderr=PIPE)
        pwd = p.stdout.read()
        print(pwd)
        return str(pwd, encoding = "utf-8")
    return ""


def handler(event, context):
    print(event)
    
    event = json.loads(event)
    if event["isBase64Encoded"]:
        req = json.loads(base64.decodestring(bytes(event['body'], encoding = "utf8")))
    else:
        req = json.loads(event['body'])
    print(req)
    cmd = ""
    if req["HttpAction"] == "FunExecPwd":
        cmd = cmd+"echo `pwd` > /tmp/%s;"
        if req["Pwd"] != "":
            cmd = cmd+"cd %s;"%req["Pwd"]
        cmd = cmd + req["Cmd"] + "; echo `pwd` > /tmp/"+req["RequestId"]
    else:
        cmd = req["Cmd"]

    p = Popen(["bash", "-c", cmd], stdout=PIPE, stderr=PIPE)
    stdout, stderr = p.communicate()
    print("stdout:%s, stderr:%s" % (stdout, stderr))

    resp_dict = {
        'RequestId': req["RequestId"], 
        'Result': "success", 
        'StdOut': str(stdout, encoding = "utf-8"),
        'StdErr': str(stderr, encoding = "utf-8"),
        'Pwd':getPwd(req),
    }
    print(json.dumps(resp_dict))

    # you can deal with your own logic here. 
    rep = {
        "isBase64Encoded": "false",
        "statusCode": "200",
        "headers": {
            "x-custom-header": "no"
        },
        "body": resp_dict
    }
    return json.dumps(rep)
