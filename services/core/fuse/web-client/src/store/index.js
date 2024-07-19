import { configureStore } from '@reduxjs/toolkit';
import { setupListeners } from '@reduxjs/toolkit/query';

import counterReducer from './counter';

export const store = configureStore({
  reducer: {
    counter: counterReducer,
  },
})

setupListeners(store.dispatch)