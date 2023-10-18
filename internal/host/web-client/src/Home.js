import React from 'react';
import { signOut } from "supertokens-auth-react/recipe/emailpassword";

export function Home() {
    async function onLogout() {
        await signOut();
        window.location.href = "/";
    }

    return (
        <>
            <h1>Home</h1>
            <button onClick={onLogout}>Logout</button>
        </>
    );
}
