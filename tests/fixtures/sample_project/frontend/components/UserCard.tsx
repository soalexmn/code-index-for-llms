/** UserCard displays a single user's details with optional action buttons. */
import React, { useState } from "react";
import { User, Role } from "../types/models";

export interface UserCardProps {
  user: User;
  onEdit?: (user: User) => void;
  onDelete?: (userId: string) => void;
  showActions?: boolean;
}

function roleBadgeColor(role: Role): string {
  switch (role) {
    case Role.Admin:  return "#e53e3e";
    case Role.Editor: return "#dd6b20";
    case Role.Viewer: return "#3182ce";
    default:          return "#718096";
  }
}

export const UserCard: React.FC<UserCardProps> = ({
  user,
  onEdit,
  onDelete,
  showActions = true,
}) => {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="user-card" data-testid={`user-card-${user.id}`}>
      <div className="user-card__header" onClick={() => setExpanded(e => !e)}>
        <span className="user-card__name">{user.displayName || user.email}</span>
        <span
          className="user-card__role-badge"
          style={{ backgroundColor: roleBadgeColor(user.role) }}
        >
          {user.role}
        </span>
      </div>
      {expanded && (
        <div className="user-card__details">
          <p>{user.email}</p>
          <p>Active: {user.isActive ? "Yes" : "No"}</p>
          <p>Joined: {new Date(user.createdAt).toLocaleDateString()}</p>
        </div>
      )}
      {showActions && (
        <div className="user-card__actions">
          {onEdit && (
            <button onClick={() => onEdit(user)}>Edit</button>
          )}
          {onDelete && (
            <button onClick={() => onDelete(user.id)}>Delete</button>
          )}
        </div>
      )}
    </div>
  );
};

export default UserCard;
