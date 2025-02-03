import { BuiltInNode, Handle, NodeTypes, Position, type Node, type NodeProps } from '@xyflow/react';
import { useStore } from './ChaosStore';

// Define clients
export type ClientNode = Node<ClientNodeData, 'client-node'>;

export type ClientNodeData = {
    userId: string,
    position: Position,
};

export function ClientNode({
    data,
}: NodeProps<ClientNode>) {

    return (
        // We add this class to use the same styles as React Flow's default nodes.
        <div className="react-flow__node-default">
            <img src="/client.svg" width="48px" />
            <div>{data.userId}</div>
            <Handle type="source" position={data.position} />
        </div>
    );
}

// Define servers
export type HomeserverNode = Node<HomeserverNodeData, 'homeserver-node'>;

export type HomeserverNodeData = {
    domain: string,
};

export function HomeserverNode({
    data,
}: NodeProps<HomeserverNode>) {
    const d = data as HomeserverNodeData;
    const serversRestarting = useStore((state) => state.serversRestarting);
    const isServerRestarting = serversRestarting.has(d.domain);
    const restart = useStore((state) => state.restart);

    return (
        // We add this class to use the same styles as React Flow's default nodes.
        <>
            <div className="react-flow__node-default">
                {data.domain && <div>{data.domain}</div>}

                <div>
                    <input type="button" value={isServerRestarting ? "Restarting" : "Restart"} disabled={isServerRestarting} onClick={() => restart(d.domain)} />
                </div>

                <Handle type="target" id="client" position={Position.Top} />
                <Handle type="target" id="federationL" position={Position.Left} />
                <Handle type="target" id="federationR" position={Position.Right} />
                <Handle type="source" id="federationL" position={Position.Left} />
                <Handle type="source" id="federationR" position={Position.Right} />
            </div>
        </>
    );
}

// Define union
export type AppNode = HomeserverNode | ClientNode;


export const AppNodeTypes = {
    'homeserver-node': HomeserverNode,
    "client-node": ClientNode,
} satisfies NodeTypes;