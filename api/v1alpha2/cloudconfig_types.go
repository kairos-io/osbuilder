package v1alpha2

type CloudConfig struct {
	Users  Users  `json:"users,omitempty"`
	Plural Plural `json:"plural,omitempty"`
}

type Users []User

func (in Users) FirstUser() *User {
	for _, user := range in {
		return &user
	}

	return nil
}

type User struct {
	Name   string `json:"name"`
	Passwd string `json:"passwd"`
}

func (in *User) GetName() *string {
	if in == nil {
		return nil
	}

	return &in.Name
}

func (in *User) GetPasswd() *string {
	if in == nil {
		return nil
	}

	return &in.Passwd
}

type Plural struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}
