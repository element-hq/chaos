import React, { useReducer } from 'react';
import { BaseEdge, Edge, EdgeLabelRenderer, getBezierPath, getSmoothStepPath, type EdgeProps } from '@xyflow/react';
import { useStore } from './ChaosStore';
import { PayloadFederationRequest } from './WebSockets';

export type FederationEdge = Edge<{ duration: number }, 'federation'>;

export type FederationEdgeData = {
    domain: string
};

export function FederationEdge({
    id,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    markerEnd,
    data,
    label,
}: EdgeProps) {
    const edgePathParams = {
        sourceX,
        sourceY,
        sourcePosition,
        targetX,
        targetY,
        targetPosition,
    };
    const edgePath = getSpecialPath(edgePathParams, sourceX < targetX ? 25 : -25);

    const fedLatencyMs = useStore((state) => state.fedLatencyMs);
    const inflightReqs = useStore((state) => state.inflightFedRequests);

    const duration = "1000ms";
    const flightBubbles = [
        <circle r="10" fill="#ff0073" key="tmp">
            <animateMotion dur={duration} repeatCount="indefinite" path={edgePath} />
        </circle>
    ];
    for (const [id, req] of inflightReqs) {

    }
    console.log("FederationEdge re-render ", flightBubbles.length, "flightBubbles ", label);
    const d = data as FederationEdgeData;
    return (
        <>
            <BaseEdge id={id} path={edgePath} markerEnd={markerEnd} label={label} />
            {flightBubbles}
        </>
    );
}



export function ClientServerEdgeLabel(props: { domain: string }) {
    const clients = useStore((state) => state.clients);
    let client = clients[props.domain] || { action: "-" };
    return (
        <>
            {client.action}
        </>
    );
}

type GetSpecialPathParams = {
    sourceX: number;
    sourceY: number;
    targetX: number;
    targetY: number;
};
const getSpecialPath = (
    { sourceX, sourceY, targetX, targetY }: GetSpecialPathParams,
    offset: number,
) => {
    const centerX = (sourceX + targetX) / 2;
    const centerY = (sourceY + targetY) / 2;

    return `M ${sourceX} ${sourceY} Q ${centerX} ${centerY + offset
        } ${targetX} ${targetY}`;
};