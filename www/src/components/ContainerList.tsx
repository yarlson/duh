import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getContainers, startContainer, stopContainer } from '../api/containers'

function formatContainerName(names: string[]): string {
    if (!names?.length) return 'Unnamed'
    const name = names[0]
    if (!name) return 'Unnamed'
    return name.replace(/^\//, '')
}

function getContainerStateLabel(state: string): string {
    switch (state) {
        case 'running':
            return 'Running - Click to stop'
        case 'exited':
            return 'Stopped - Click to start'
        case 'starting':
            return 'Starting...'
        case 'stopping':
            return 'Stopping...'
        default:
            return `${state} state`
    }
}

export function ContainerList() {
    const queryClient = useQueryClient()

    const { data: containers, isLoading, error } = useQuery({
        queryKey: ['containers'],
        queryFn: getContainers,
        refetchInterval: 5000, // Refresh every 5 seconds
    })

    const startMutation = useMutation({
        mutationFn: startContainer,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['containers'] })
        },
    })

    const stopMutation = useMutation({
        mutationFn: stopContainer,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['containers'] })
        },
    })

    if (isLoading) return <div className="loading">Loading...</div>
    if (error) {
        const message = error instanceof Error ? error.message : 'Unknown error'
        return (
            <div className="error">
                Error: {message}
                <div className="error__retry">The application will automatically retry when the server is available.</div>
            </div>
        )
    }

    return (
        <div className="container-list">
            {containers?.map((container) => (
                <div key={container.id} className="container-card">
                    <div className="container-card__header">
                        <h3 className="container-card__name">{formatContainerName(container.names)}</h3>
                        <span className={`container-card__status container-card__status--${container.state}`}>
                            {container.state}
                        </span>
                    </div>
                    <div className="container-card__content">
                        <p className="container-card__image">{container.image}</p>
                        {container.stats && (
                            <div className="container-card__stats">
                                <p>
                                    CPU: {container.stats.cpu_stats.usage.toFixed(1)}%
                                </p>
                                <p>
                                    Memory: {formatBytes(container.stats.memory_stats.usage)} / {formatBytes(container.stats.memory_stats.limit)}
                                </p>
                            </div>
                        )}
                    </div>
                    <div className="container-card__footer">
                        <label className="switch">
                            <input
                                type="checkbox"
                                checked={container.state === 'running'}
                                disabled={
                                    startMutation.isPending ||
                                    stopMutation.isPending ||
                                    container.state === 'starting' ||
                                    container.state === 'stopping'
                                }
                                onChange={() => {
                                    if (container.state === 'running') {
                                        stopMutation.mutate(container.id)
                                    } else if (container.state === 'exited') {
                                        startMutation.mutate(container.id)
                                    }
                                }}
                                aria-label={getContainerStateLabel(container.state)}
                            />
                            <span className="switch__slider"></span>
                        </label>
                    </div>
                </div>
            ))}
        </div>
    )
}

function formatBytes(bytes: number): string {
    const units = ['B', 'KB', 'MB', 'GB']
    let size = bytes
    let unitIndex = 0

    while (size >= 1024 && unitIndex < units.length - 1) {
        size /= 1024
        unitIndex++
    }

    return `${size.toFixed(1)} ${units[unitIndex]}`
} 
