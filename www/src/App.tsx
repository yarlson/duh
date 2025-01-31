import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ContainerList } from './components/ContainerList'
import './styles.css'

const queryClient = new QueryClient()

function App() {
    return (
        <QueryClientProvider client={queryClient}>
            <div className="dashboard">
                <div className="dashboard__header">
                    <h1 className="dashboard__title">Docker Containers</h1>
                </div>
                <ContainerList />
            </div>
        </QueryClientProvider>
    )
}

export default App
