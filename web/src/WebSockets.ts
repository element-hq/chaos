

export type WebSocketMessage = {
    ID: string
    Type: string
    Payload: Record<string,any>
}

export class ChaosWebsocket extends EventTarget {
    ws!: WebSocket;

    connect(url: string): Promise<void> {
        this.ws = new WebSocket(url);
        return new Promise<void>((resolve) => {
            this.ws.addEventListener("open", () => {
                console.log("WS open");
                resolve();
            });
            this.ws.addEventListener("error", this.onWsError.bind(this));
            this.ws.addEventListener("close", this.onWsClose.bind(this));
            this.ws.addEventListener("message", this.onWsMessage.bind(this));
        });
    }

    onWsClose(_: CloseEvent) {}
    onWsError(_: Event) {}
    onWsMessage(ev: MessageEvent) {
        const msg = JSON.parse(ev.data) as WebSocketMessage;
        switch (msg.Type) {
            case "PayloadConfig":
                this.dispatchEvent(new CustomEvent("PayloadConfig", {detail: msg.Payload}));
                break;
            case "PayloadWorkerAction":
                this.dispatchEvent(new CustomEvent("PayloadWorkerAction", {detail: msg.Payload}));
                break;
            case "PayloadTickGeneration":
                this.dispatchEvent(new CustomEvent("PayloadTickGeneration", {detail: msg.Payload}));
                break;
            case "PayloadConvergence":
                this.dispatchEvent(new CustomEvent("PayloadConvergence", {detail: msg.Payload}));
                break;
            case "PayloadNetsplit":
                this.dispatchEvent(new CustomEvent("PayloadNetsplit", {detail: msg.Payload}));
                break;
            case "PayloadFederationRequest":
                msg.Payload.ID = msg.ID;
                this.dispatchEvent(new CustomEvent("PayloadFederationRequest", {detail: msg.Payload}));
                break;
            case "PayloadRestart":
                this.dispatchEvent(new CustomEvent("PayloadRestart", {detail: msg.Payload}));
                break;
        }
    }

    start() {
        this.ws.send(JSON.stringify({
            Begin: true,
        }));
    }
    testConvergence() {
        this.ws.send(JSON.stringify({
            CheckConvergence: true,
        }));
    }
    setNetsplit(isNetsplit: boolean) {
        this.ws.send(JSON.stringify({
            Netsplit: isNetsplit,
        }));
    }
    setRestart(domain: string) {
        this.ws.send(JSON.stringify({
            RestartServers: [domain],
        }));
    }
}

export type PayloadNetsplit = {
    Started: boolean
}
export type PayloadConvergence = {
    State: string
    Error: string
}
export type PayloadTickGeneration = {
    Number: number,
    Joins: number,
    Sends: number,
    Leaves: number
}
export type PayloadWorkerAction = {
    UserID: string, 
    RoomID: string, 
    Action: string, 
    Body: string 
}
export type PayloadConfig = {
    WorkerUserIDs: Array<string>, 
    Config: Record<string, any>
}
export type PayloadFederationRequest = {
    ID: string, // msg
    Method: string,
    URL: string,
    Body: Record<string,any>,
    Blocked: boolean
}
export type PayloadRestart = {
    Domain: string,
    Finished: boolean,
}