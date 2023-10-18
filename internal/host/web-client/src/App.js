import React from 'react';
import {
  BrowserRouter,
  Routes,
  Route,
} from "react-router-dom";

import { SuperTokensWrapper } from "supertokens-auth-react";
import { getSuperTokensRoutesForReactRouterDom } from "supertokens-auth-react/ui";
import { EmailPasswordPreBuiltUI } from "supertokens-auth-react/recipe/emailpassword/prebuiltui";
import { SessionAuth } from "supertokens-auth-react/recipe/session";
import * as reactRouterDom from "react-router-dom";

import {Home} from "./Home";

class App extends React.Component {
    render() {
        return (
            <SuperTokensWrapper>
                <BrowserRouter>
                    <Routes>
                        {/*This renders the login UI on the /auth route*/}
                        {getSuperTokensRoutesForReactRouterDom(reactRouterDom, [EmailPasswordPreBuiltUI])}
                        {/*Your app routes*/}
 
                        <Route path="/" element={
                            <SessionAuth>
                                <Home/>
                            </SessionAuth>
                        }/>
                    </Routes>
                </BrowserRouter>
            </SuperTokensWrapper>
        );
    }
}

export default App;