import React, { memo } from "react";
import { formatContainerName } from "../utils/formatters";

const ContainerCard = memo(function ContainerCard({
  container,
  onToggle,
  isLoading,
}: {
  container: any;
  onToggle: (id: string, currentState: string) => void;
  isLoading: boolean;
}) {
  // Compute displayState to override the current container state if the toggle is pending.
  const displayState = isLoading
    ? container.state === "running"
      ? "stopping"
      : container.state === "exited"
        ? "starting"
        : container.state
    : container.state;

  return (
    <div key={container.id} className="container-card">
      <div className="container-card__header">
        <h3 className="container-card__name">
          {formatContainerName(container.names)}
        </h3>
        <span
          className={`container-card__status container-card__status--${displayState}`}
        >
          {displayState}
        </span>
      </div>
      {/* The rest of ContainerCard remains unchanged. */}
    </div>
  );
});

export default ContainerCard;
