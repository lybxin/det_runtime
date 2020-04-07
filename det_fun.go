// +build lambda

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime_det/common/action"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func returnRsp(resp *action.CmdResponse) (events.APIGatewayProxyResponse, error) {
	respJson, err := json.Marshal(resp)
	if err != nil {
		fmt.Printf("json marshal rsp failed:%+v\n", resp)
		return events.APIGatewayProxyResponse{Body: `{"Result":"json marshal rsp failed"}`, StatusCode: 200}, nil
	}
	return events.APIGatewayProxyResponse{Body: string(respJson), StatusCode: 200}, nil
}

func HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	fmt.Printf("Processing request data for request %+v\n ctx:%+v", request, ctx)

	req := &action.CmdRequest{}
	if err := json.Unmarshal([]byte(request.Body), req); err != nil {
		return returnRsp(action.MakeResp("para is invalid json format"))
	}
	httpActionFun, ok := action.HttpActionTable[action.HttpAction(req.HttpAction)]
	if !ok {
		return returnRsp(action.MakeResp("invalid action"))
	}

	resp := httpActionFun(req)
	fmt.Printf("req:%s,\n,resp:%s\n", req, resp)
	return returnRsp(resp)
}

func main() {
	lambda.Start(HandleRequest)
}
