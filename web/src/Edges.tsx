import React from 'react';
import { BaseEdge, Edge, getBezierPath, getSmoothStepPath, type EdgeProps } from '@xyflow/react';
import { useStore } from './ChaosStore';

export type AnimatedSVGEdge = Edge<{ duration: number }, 'animatedSvg'>;

export type AnimatedSVGEdgeData = {
    duration: string
};

export function AnimatedSVGEdge({
    id,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    data,
}: EdgeProps) {
    const [edgePath] = getSmoothStepPath({
        sourceX,
        sourceY,
        sourcePosition,
        targetX,
        targetY,
        targetPosition,
    });
    const animData = data as AnimatedSVGEdgeData;
    const duration = animData.duration || "1s";

    return (
        <>
            <BaseEdge id={id} path={edgePath} />
            <circle r="10" fill="#ff0073">
                <animateMotion dur={duration} repeatCount="indefinite" path={edgePath} />
            </circle>
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