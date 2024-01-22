import React, { useState } from "react";
import { NavLink } from "react-router-dom";
import { Icon, Tag, Popover } from "@blueprintjs/core";
import "./../index.css";

export default function MainAppBar() {
   const [isMenuOpen, setIsMenuOpen] = useState(false);
   const [notificationCount, setNotificationCount] = useState(5);

   const handleNotificationClick = () => {
      setNotificationCount(0); // Reset count when clicked
   };
   const handleMenuClick = () => {
      setIsMenuOpen(!isMenuOpen);
   };

   const closeMenu = () => {
      setIsMenuOpen(false);
   };

   return (
      <div className="header">
         <div className="header-left">
            <div className="menu-icon" onClick={handleMenuClick}>
               <Icon icon="menu" color="#edeff2" />

               {isMenuOpen && (
                  <div className="menu">
                     <div className="menu-row">
                        <div className="menu-left">
                           <NavLink to="/" onClick={closeMenu}>
                              <Icon
                                 icon="series-filtered"
                                 className="icon-menu"
                                 color="#5F6B7C"
                              />
                              Metrics{" "}
                           </NavLink>
                        </div>
                        <div className="menu-right">
                           <Icon
                              icon="key-command"
                              className="icon-cmd"
                              color="#8F99A8"
                           />
                           M
                        </div>
                     </div>
                     <div className="menu-row">
                        <div className="menu-left">
                           <NavLink to="/service-registry" onClick={closeMenu}>
                              <Icon
                                 icon="one-to-many"
                                 className="icon-menu"
                                 color="#5F6B7C"
                              />
                              Services
                           </NavLink>
                        </div>
                        <div className="menu-right">
                           <Icon
                              icon="key-command"
                              className="icon-cmd"
                              color="#8F99A8"
                           />
                           X
                        </div>
                     </div>

                     <div className="menu-row">
                        <div className="menu-left">
                           <NavLink to="/key-values" onClick={closeMenu}>
                              <Icon
                                 icon="heatmap"
                                 className="icon-menu"
                                 color="#5F6B7C"
                              />
                              Key/Values
                           </NavLink>
                        </div>
                        <div className="menu-right">
                           <Icon
                              icon="key-command"
                              className="icon-cmd"
                              color="#8F99A8"
                           />
                           K
                        </div>
                     </div>
                     <div className="divider" />
                     <div className="menu-left">
                        <Icon
                           icon="cog"
                           color="#5F6B7C"
                           className="icon-menu"
                           onClick={closeMenu}
                        />
                        Settings
                     </div>
                  </div>
               )}

               <span className="logo-text">{"{blueprint}"}</span>
            </div>
         </div>
         <div className="header-right">
            {/* ----- TODO: Get this Notify Badge working */}
            <Popover
               content={<div>Notify Content</div>}
               isOpen={notificationCount > 0}
               minimal
               onInteraction={handleNotificationClick}
            >
               <Tag intent="danger">{notificationCount}</Tag>
               <Icon icon="notifications" className="notify" color="#edeff2" />
            </Popover>
         </div>
      </div>
   );
}
