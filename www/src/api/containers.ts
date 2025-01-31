import { Container } from '../types'

const API_BASE = '/api'

export async function getContainers(): Promise<Container[]> {
    const response = await fetch(`${API_BASE}/containers`)
    if (!response.ok) {
        throw new Error('Failed to fetch containers')
    }
    return response.json()
}

export async function startContainer(id: string): Promise<void> {
    const response = await fetch(`${API_BASE}/containers/${id}?action=start`, {
        method: 'POST',
    })
    if (!response.ok) {
        throw new Error('Failed to start container')
    }
}

export async function stopContainer(id: string): Promise<void> {
    const response = await fetch(`${API_BASE}/containers/${id}?action=stop`, {
        method: 'POST',
    })
    if (!response.ok) {
        throw new Error('Failed to stop container')
    }
} 