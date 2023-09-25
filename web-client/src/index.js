import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import reportWebVitals from './reportWebVitals';

import {
  createBrowserRouter,
  RouterProvider,
} from "react-router-dom";

import SuperTokens, { SuperTokensWrapper } from "supertokens-auth-react";
import ThirdPartyEmailPassword, {Github, Google, Facebook, Apple} from "supertokens-auth-react/recipe/thirdpartyemailpassword";
import Session from "supertokens-auth-react/recipe/session";

const router = createBrowserRouter([
  {
    path: "/",
    element: <App />,
  },
]);

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();

// Used to login users to our application
SuperTokens.init({
  appInfo: {
      // learn more about this on https://supertokens.com/docs/thirdpartyemailpassword/appinfo
      appName: "<YOUR_APP_NAME>",
      apiDomain: "<YOUR_API_DOMAIN>",
      websiteDomain: "<YOUR_WEBSITE_DOMAIN>",
      apiBasePath: "/auth",
      websiteBasePath: "/auth"
  },
  recipeList: [
      ThirdPartyEmailPassword.init({
          signInAndUpFeature: {
              providers: [
                  Github.init(),
                  Google.init(),
                  Facebook.init(),
                  Apple.init(),
              ]
          }
      }),
      Session.init()
  ]
});
