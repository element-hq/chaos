import { BuiltInNode, Handle, NodeTypes, Position, type Node, type NodeProps } from '@xyflow/react';
import { useStore } from './ChaosStore';

// Define clients
export type ClientNode = Node<ClientNodeData, 'client-node'>;

export type ClientNodeData = {
    domain: string,
};

export function ClientNode({
    data,
}: NodeProps<ClientNode>) {
    const clients = useStore((state) => state.clients);
    let client = clients[data.domain];
    if (!client) {
        client = {
            userId: "-",
            action: "-",
        };
    }

    return (
        // We add this class to use the same styles as React Flow's default nodes.
        <div className="react-flow__node-default">
            <img src="/client.svg" width="48px" />
            <div>{client.userId}</div>
            <Handle type="source" position={Position.Bottom} />
        </div>
    );
}

// Define servers
export type HomeserverNode = Node<HomeserverNodeData, 'homeserver-node'>;

export type HomeserverNodeData = {
    label: string,
    isRestarting: boolean,
};

export function HomeserverNode({
    data,
}: NodeProps<HomeserverNode>) {
    const d = data as HomeserverNodeData;
    return (
        // We add this class to use the same styles as React Flow's default nodes.
        <div className="react-flow__node-default">
            {data.label && <div>{data.label}</div>}

            <div>
                <input type="button" value={d.isRestarting ? "Restarting" : "Restart"} disabled={d.isRestarting} />
            </div>

            <Handle type="target" id="client" position={Position.Top} />
            <Handle type="target" id="federation" position={Position.Left} />
            <Handle type="source" position={Position.Right} />
        </div>
    );
}

// Define union
export type AppNode = HomeserverNode | ClientNode;


export const AppNodeTypes = {
    'homeserver-node': HomeserverNode,
    "client-node": ClientNode,
} satisfies NodeTypes;