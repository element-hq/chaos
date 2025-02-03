import { create } from 'zustand';
import { type AppNode } from "./Nodes";
import { ChaosWebsocket, PayloadConfig, PayloadConvergence, PayloadFederationRequest, PayloadNetsplit, PayloadRestart, PayloadTickGeneration, PayloadWorkerAction } from './WebSockets';
import { addEdge, applyEdgeChanges, applyNodeChanges, Connection, Edge, EdgeChange, NodeChange } from '@xyflow/react';
import { AppEdge, ClientServerEdgeLabel } from './Edges';

export type ChaosStore = {
    convergenceState: string
    isNetsplit: boolean
    started: boolean
    fedLatencyMs: number
    inflightFedRequests: Map<string, {
        payload: PayloadFederationRequest,
        start: number,
    }>

    // for now this is domain => client so we only support 1 client TODO allow multiple
    clients: Record<string, {
        userId: string,
        action: string,
    }>
    tickNumber: number

    connectedToRemoteServer: boolean
    serversRestarting: Set<string>

    nodes: AppNode[]
    edges: AppEdge[]

    onConnected: (payload: PayloadConfig) => void
    onWorkerAction: (payload: PayloadWorkerAction) => void
    onTickGeneration: (payload: PayloadTickGeneration) => void
    onConvergenceUpdate: (payload: PayloadConvergence) => void
    onNetsplit: (payload: PayloadNetsplit) => void
    onFederationRequest: (payload: PayloadFederationRequest) => void
    onServerRestart: (payload: PayloadRestart) => void


    connect: (wsAddr: string) => void
    start: () => void
    restart: (hsName: string) => void
    netsplitToggle: () => void
    testConvergence: () => void

    onNodesChange: (changes: NodeChange<AppNode>[]) => void
    onEdgesChange: (changes: EdgeChange<AppEdge>[]) => void
    onConnect: (connection: Edge | Connection) => void
    setNodes: (nodes: AppNode[]) => void
    setEdges: (edges: AppEdge[]) => void

}


function sleep(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

let ws = new ChaosWebsocket();

export const useStore = create<ChaosStore>()((set, get) => ({
    convergenceState: "-",
    started: false,
    isNetsplit: false,
    connectedToRemoteServer: false,
    tickNumber: 0,
    fedLatencyMs: 1000,
    clients: {},
    inflightFedRequests: new Map(),
    serversRestarting: new Set<string>(),
    nodes: [
        {
            id: 'hs1', type: 'homeserver-node', position: { x: -300, y: 100 },
            data: { domain: "hs1" }
        },
        {
            id: 'hs2', type: 'homeserver-node', position: { x: 300, y: 100 },
            data: { domain: "hs2" }
        },
        { id: "client1", type: "client-node", position: { x: -300, y: -100 }, data: { domain: "hs1" } },
        { id: "client2", type: "client-node", position: { x: 300, y: -100 }, data: { domain: "hs2" } },
    ],
    edges: [
        { id: 'hs1hs2', source: 'hs1', target: 'hs2', sourceHandle: "federationR", targetHandle: "federationL", label: "hs1", type: "federation", data: { domain: "hs1" } },
        { id: 'hs2hs1', source: 'hs2', target: 'hs1', sourceHandle: "federationL", targetHandle: "federationR", label: "hs2", type: "federation", data: { domain: "hs2" } },
        { id: "hs1-client1", source: "client1", target: "hs1", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs1" /> },
        { id: "hs2-client1", source: "client2", target: "hs2", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs2" /> },
    ],

    // Server-received actions (mapped from WS payloads)
    // -------------------------------------------------

    onConnected: (payload: PayloadConfig) => {
        if (payload.Config.Test.NumUsers !== 2 || payload.Config.Homeservers.length !== 2) {
            console.error("Incompatible Chaos configuration, only 2 HS with 1 user each is supported currently.");
            console.error("Configuration received:", payload.Config);
        }
        if (payload.Config.Test.FederationDelayMs) {
            set({
                fedLatencyMs: payload.Config.Test.FederationDelayMs,
            });
        }
        const clients: Record<string, {
            userId: string,
            action: string,
        }> = {};
        for (const userId of payload.WorkerUserIDs) {
            const domain = userId.split(":")[1]
            clients[domain] = {
                userId: userId,
                action: "-",
            };
        }
        console.log("setting", clients);
        set({
            clients: clients,
        });
    },
    onWorkerAction: (payload: PayloadWorkerAction) => {
        const domain = payload.UserID.split(":")[1];
        const client = get().clients[domain];
        client.action = `${payload.Action} ${payload.Body ? payload.Body : ""}`;
        set({
            clients: {
                ...get().clients,
                domain: client,
            }
        })
    },
    onTickGeneration: (payload: PayloadTickGeneration) => {
        set({
            tickNumber: payload.Number
        });
    },
    onConvergenceUpdate: (payload: PayloadConvergence) => {
        if (payload.Error) {
            set({
                convergenceState: "ERROR: " + payload.Error,
            });
        } else {
            set({
                convergenceState: payload.State,
            });
        }
    },
    onNetsplit: (payload: PayloadNetsplit) => {
        set({
            isNetsplit: payload.Started,
        });
    },
    onFederationRequest: (payload: PayloadFederationRequest) => {
        useStore.setState((prev) => {
            console.log(payload.ID, payload.URL, "existing: ", prev.inflightFedRequests.size);
            const copy = new Map(prev.inflightFedRequests);
            copy.set(payload.ID, {
                payload: payload,
                start: Date.now(),
            });
            setTimeout(() => {
                useStore.setState((prev) => {
                    const copy = new Map(prev.inflightFedRequests);
                    copy.delete(payload.ID);
                    return {
                        inflightFedRequests: copy,
                    };
                })
            }, prev.fedLatencyMs);
            return {
                inflightFedRequests: copy,
            };
        });
    },
    onServerRestart: (payload: PayloadRestart) => {
        useStore.setState((prev) => {
            const copy = new Set<string>(prev.serversRestarting);
            if (payload.Finished) {
                copy.delete(payload.Domain);
            } else {
                copy.add(payload.Domain);
            }
            console.log("Servers restarting: ", Array.from(copy));
            return {
                serversRestarting: copy,
            };
        });
    },



    // End-user issued actions
    // -----------------------

    connect: async (wsAddr: string): Promise<void> => {
        console.log("connect ", wsAddr);
        ws.addEventListener("PayloadConfig", (ev: unknown) => {
            set({
                connectedToRemoteServer: true,
            });
            get().onConnected((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadWorkerAction", (ev: unknown) => {
            get().onWorkerAction((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadTickGeneration", (ev: unknown) => {
            get().onTickGeneration((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadConvergence", (ev: unknown) => {
            get().onConvergenceUpdate((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadNetsplit", (ev: unknown) => {
            get().onNetsplit((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadFederationRequest", (ev: unknown) => {
            get().onFederationRequest((ev as CustomEvent).detail);
        });
        ws.addEventListener("PayloadRestart", (ev: unknown) => {
            get().onServerRestart((ev as CustomEvent).detail);
        });
        await ws.connect(wsAddr);
    },
    start: async (): Promise<void> => {
        console.log("start");
        ws.start();
    },
    restart: (hsName: string): void => {
        console.log("restart ", hsName);
        ws.setRestart(hsName);
    },
    netsplitToggle: (): void => {
        console.log("netsplitToggle");
        ws.setNetsplit(!get().isNetsplit);
    },
    testConvergence: (): void => {
        ws.testConvergence();
    },

    // Reactflow functions
    // -------------------

    onNodesChange: (changes) => {
        set({
            nodes: applyNodeChanges(changes, get().nodes),
        });
    },
    onEdgesChange: (changes) => {
        set({
            edges: applyEdgeChanges(changes, get().edges),
        });
    },
    onConnect: (connection) => {
        set({
            edges: addEdge(connection, get().edges),
        });
    },
    setNodes: (nodes) => {
        set({ nodes });
    },
    setEdges: (edges) => {
        set({ edges });
    },
}));
