import Welcome from './windows/Welcome'
import Workspace from './windows/Workspace'

function App() {
  const hash = window.location.hash.replace('#', '')
  if (hash === 'welcome') {
    return <Welcome />
  }
  return <Workspace />
}

export default App
