package v1alpha2

type CloudConfig struct {
	Users  Users  `json:"users,omitempty" yaml:"users,omitempty"`
	Plural Plural `json:"plural,omitempty" yaml:"plural,omitempty"`
}

type Users []User

func (in Users) FirstUser() *User {
	for _, user := range in {
		return &user
	}

	return nil
}

type User struct {
	Name   string `json:"name" yaml:"name"`
	Passwd string `json:"passwd" yaml:"passwd"`
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
	Token string `json:"token" yaml:"token"`
	URL   string `json:"url" yaml:"url"`
}
