import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './styles/main.css'
import { copy } from './copy'

document.title = copy.appName

createApp(App).use(router).mount('#app')
