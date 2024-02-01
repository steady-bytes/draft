import React, { useState, useEffect, useRef } from "react";
import { FaChevronDown, FaChevronUp } from "react-icons/fa";

const Search = ({ options, onSelect }) => {
  const [isOpen, setIsOpen] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const dropdownRef = useRef(null);

  useEffect(() => {
    document.addEventListener("click", handleClickOutside);
    return () => {
      document.removeEventListener("click", handleClickOutside);
    };
  }, []);

  const handleClickOutside = (event) => {
    if (
      dropdownRef.current &&
      !dropdownRef.current.contains(event.target) &&
      event.target.className !== "selected-option"
    ) {
      setIsOpen(false);
    }
  };

  const toggleDropdown = (event) => {
    event.stopPropagation(); // Prevent the event from propagating to the parent div
    setIsOpen(!isOpen);
  };

  const handleOptionClick = (option) => {
    onSelect(option);
    setIsOpen(false);
  };

  const filteredOptions = options.filter((option) =>
    option.label.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="custom-select">
      <div className="selected-option" onClick={toggleDropdown}>
        {searchTerm ? searchTerm : "Select..."}
        {isOpen ? (
          <FaChevronUp onClick={toggleDropdown} />
        ) : (
          <FaChevronDown onClick={toggleDropdown} />
        )}
      </div>
      {isOpen && (
        <div className="options" ref={dropdownRef}>
          <div className="dropdown-list">
            {filteredOptions.map((option) => (
              <div
                key={option.value}
                className="option"
                onClick={() => handleOptionClick(option)}
              >
                {option.label}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

export default Search;
