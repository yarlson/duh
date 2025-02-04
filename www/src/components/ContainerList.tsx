import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, useCallback, useMemo, memo } from "react";
import {
  getContainers,
  startContainer,
  stopContainer,
} from "../api/containers";

function formatContainerName(names: string[]): string {
  if (!names?.length) return "Unnamed";
  const name = names[0];
  if (!name) return "Unnamed";
  return name.replace(/^\//, "");
}

function getContainerStateLabel(state: string): string {
  switch (state) {
    case "running":
      return "Running - Click to stop";
    case "exited":
      return "Stopped - Click to start";
    case "starting":
      return "Starting...";
    case "stopping":
      return "Stopping...";
    default:
      return `${state} state`;
  }
}

// Add this new component for the stats skeleton
const StatsSkeleton = memo(function StatsSkeleton() {
  return (
    <div className="container-card__stats container-card__stats--loading">
      <p>
        CPU: <span className="skeleton-text">--.--%</span>
      </p>
      <p>
        Memory: <span className="skeleton-text">--.- MB / --.- GB</span>
      </p>
    </div>
  );
});

// Memoized container card component
const ContainerCard = memo(function ContainerCard({
  container,
  onToggle,
  isLoading,
}: {
  container: any;
  onToggle: (id: string, currentState: string) => void;
  isLoading: boolean;
}) {
  return (
    <div key={container.id} className="container-card">
      <div className="container-card__header">
        <h3 className="container-card__name">
          {formatContainerName(container.names)}
        </h3>
        <span
          className={`container-card__status container-card__status--${container.state}`}
        >
          {container.state}
        </span>
      </div>
      <div className="container-card__content">
        <p className="container-card__image">{container.image}</p>
        {container.state === "running" &&
          (container.stats ? (
            <div className="container-card__stats">
              <p>CPU: {container.stats.cpu_stats.usage.toFixed(1)}%</p>
              <p>
                Memory: {formatBytes(container.stats.memory_stats.usage)} /{" "}
                {formatBytes(container.stats.memory_stats.limit)}
              </p>
            </div>
          ) : (
            <StatsSkeleton />
          ))}
      </div>
      <div className="container-card__footer">
        <label className="switch">
          <input
            type="checkbox"
            checked={container.state === "running"}
            disabled={
              isLoading ||
              container.state === "starting" ||
              container.state === "stopping"
            }
            onChange={() => onToggle(container.id, container.state)}
            aria-label={getContainerStateLabel(container.state)}
          />
          <span className="switch__slider"></span>
        </label>
      </div>
    </div>
  );
});

export function ContainerList() {
  const [searchTerm, setSearchTerm] = useState("");
  const [pendingIds, setPendingIds] = useState<string[]>([]);

  const queryClient = useQueryClient();

  const {
    data: containers,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["containers"],
    queryFn: getContainers,
    refetchInterval: 5000,
  });

  const startMutation = useMutation({
    mutationFn: startContainer,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers"] });
    },
  });

  const stopMutation = useMutation({
    mutationFn: stopContainer,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers"] });
    },
  });

  // Memoized filter function - moved outside component for better performance
  const filterContainers = useCallback(
    (containers: any[], searchTerm: string) => {
      // Remove debounce for empty search to make clearing instant
      if (!searchTerm.trim()) return containers;
      const normalized = searchTerm.toLowerCase();
      return containers?.filter((container) =>
        formatContainerName(container.names).toLowerCase().includes(normalized),
      );
    },
    [],
  );

  // Memoized filtered containers - now using searchTerm directly
  const filteredContainers = useMemo(
    () => filterContainers(containers || [], searchTerm),
    [containers, searchTerm, filterContainers],
  );

  // Updated memoized toggle handler - track pending container id individually.
  const handleToggle = useCallback(
    (id: string, currentState: string) => {
      // Add the container id to pendingIds
      setPendingIds((prev) => [...prev, id]);
      if (currentState === "running") {
        stopMutation.mutate(id, {
          onSettled: () => {
            // Remove container id once mutation is settled
            setPendingIds((prev) =>
              prev.filter((pendingId) => pendingId !== id),
            );
          },
        });
      } else if (currentState === "exited") {
        startMutation.mutate(id, {
          onSettled: () => {
            // Remove container id once mutation is settled
            setPendingIds((prev) =>
              prev.filter((pendingId) => pendingId !== id),
            );
          },
        });
      }
    },
    [startMutation, stopMutation],
  );

  if (isLoading) return <div className="loading">Loading...</div>;
  if (error) {
    const message = error.message;
    return (
      <div className="error">
        Error: {message}
        <div className="error__retry">
          The application will automatically retry when the server is available.
        </div>
      </div>
    );
  }

  return (
    <div className="container-list">
      <div className="container-list__search-container">
        <input
          type="text"
          placeholder="Search containers..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          className="container-list__search-input"
          autoComplete="off"
          spellCheck="false"
        />
      </div>
      {filteredContainers?.map((container) => (
        <ContainerCard
          key={container.id}
          container={container}
          onToggle={handleToggle}
          // Use individual pending state for each container instead of a global loading indicator
          isLoading={pendingIds.includes(container.id)}
        />
      ))}
    </div>
  );
}

function formatBytes(bytes: number): string {
  const units = ["B", "KB", "MB", "GB"];
  let size = bytes;
  let unitIndex = 0;

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }

  return `${size.toFixed(1)} ${units[unitIndex]}`;
}
