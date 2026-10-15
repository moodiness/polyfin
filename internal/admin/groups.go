package admin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// groupSummaryJSON is a user group as the groups list shows it.
type groupSummaryJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"memberCount"`
}

// groupJSON is a user group with the settings it holds, named as in a
// user's JSON.
type groupJSON struct {
	groupSummaryJSON
	limitsJSON
}

func newGroupJSON(group accounts.Group) groupJSON {
	return groupJSON{
		groupSummaryJSON: groupSummaryJSON{ID: group.ID.String(), Name: group.Name, MemberCount: group.Members},
		limitsJSON:       newLimitsJSON(group.Limits),
	}
}

// groupMemberJSON is a user in a group, and whether they follow it.
type groupMemberJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FollowGroup bool   `json:"followGroup"`
}

func (h *handler) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.Accounts.Groups(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]groupSummaryJSON, 0, len(groups))
	for _, group := range groups {
		result = append(result, groupSummaryJSON{ID: group.ID.String(), Name: group.Name, MemberCount: group.Members})
	}
	writeJSON(w, http.StatusOK, result)
}

// createGroup creates a group with the settings of a user Create a user
// makes, or those of the user copyFrom names.
func (h *handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string  `json:"name"`
		CopyFrom *string `json:"copyFrom"`
	}
	if !decode(w, r, &body) {
		return
	}
	var copyFrom *accounts.ID
	if body.CopyFrom != nil && *body.CopyFrom != "" {
		id, err := accounts.ParseID(*body.CopyFrom)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_copy_from")
			return
		}
		copyFrom = &id
	}
	group, err := h.Accounts.CreateGroup(r.Context(), body.Name, copyFrom)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.GroupCreated(r.Context(), group)
	writeJSON(w, http.StatusCreated, newGroupJSON(group))
}

func (h *handler) group(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	group, err := h.Accounts.Group(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newGroupJSON(group))
}

// updateGroup changes a group's name and the settings it holds, which
// reach the members who follow it at once.
func (h *handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Name *string `json:"name"`
		limitChanges
	}
	if !decode(w, r, &body) {
		return
	}
	limits, ok := body.changes(w)
	if !ok {
		return
	}
	changes := accounts.GroupChanges{Name: body.Name, Limits: limits}
	group, err := h.Accounts.UpdateGroup(r.Context(), id, changes)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if sections := changes.Sections(); len(sections) > 0 {
		h.Activity.GroupChanged(r.Context(), group, sections)
	}
	writeJSON(w, http.StatusOK, newGroupJSON(group))
}

// deleteGroup deletes a group; its members keep its values as their own.
func (h *handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	group, err := h.Accounts.DeleteGroup(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.GroupDeleted(r.Context(), group.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) writeGroupMembers(w http.ResponseWriter, r *http.Request, id accounts.ID, status int) {
	members, err := h.Accounts.GroupMembers(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]groupMemberJSON, 0, len(members))
	for _, member := range members {
		result = append(result, groupMemberJSON{ID: member.ID.String(), Name: member.Name, FollowGroup: member.FollowGroup})
	}
	writeJSON(w, status, result)
}

func (h *handler) groupMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	h.writeGroupMembers(w, r, id, http.StatusOK)
}

// addGroupMembers has the users userIds names join the group and follow
// it, all of them or none, and answers with the group's members.
func (h *handler) addGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	users := make([]accounts.ID, 0, len(body.UserIDs))
	for _, raw := range body.UserIDs {
		user, err := accounts.ParseID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		users = append(users, user)
	}
	group, err := h.Accounts.Group(r.Context(), id)
	if err == nil {
		// The groups they leave, for the activity log.
		var everyone []accounts.User
		everyone, err = h.Accounts.Users(r.Context())
		before := make(map[accounts.ID]accounts.User, len(everyone))
		for _, user := range everyone {
			before[user.ID] = user
		}
		if err == nil {
			var joined []accounts.User
			joined, err = h.Accounts.AddGroupMembers(r.Context(), id, users)
			for _, user := range joined {
				if previous, ok := before[user.ID]; ok && previous.GroupID != nil {
					h.Activity.GroupMemberRemoved(r.Context(), user, h.groupName(r, previous.GroupID))
				}
				h.Activity.GroupMemberAdded(r.Context(), user, group.Name)
			}
		}
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeGroupMembers(w, r, id, http.StatusOK)
}
