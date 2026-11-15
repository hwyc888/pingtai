import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './styles/main.css'
import favicon from '../assets/ai-DWjrPa2g.ico'

const icon = document.createElement('link')
icon.rel = 'icon'
icon.href = favicon
document.head.appendChild(icon)

createApp(App).use(router).mount('#app')
