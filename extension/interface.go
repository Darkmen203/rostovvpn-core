package extension

import (
	"fmt"

	"github.com/Darkmen203/rostovvpn-core/v2/db"
	"github.com/Darkmen203/rostovvpn-core/v2/service_manager"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
)

var (
	allExtensionsMap     = make(map[string]ExtensionFactory)
	enabledExtensionsMap = make(map[string]*Extension)
)

func RegisterExtension(factory ExtensionFactory) error {
	if _, ok := allExtensionsMap[factory.Id]; ok {
		err := fmt.Errorf("Extension with ID %s already exists", factory.Id)
		log.Warn(err)
		return err
	}

	allExtensionsMap[factory.Id] = factory

	return nil
}

func isEnable(id string) bool {
	table := db.GetTable[extensionData]()
	extdata, err := table.Get(id)
	if err != nil {
		return false
	}
	return extdata.Enable
}

func loadExtension(factory ExtensionFactory) error {
	if !isEnable(factory.Id) {
		return fmt.Errorf("Extension with ID %s is not enabled", factory.Id)
	}
	extension := factory.Builder()
	extension.init(factory.Id)

	enabledExtensionsMap[factory.Id] = &extension

	return nil
}

type extensionService struct{}

func (s *extensionService) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	return s.start()
}

func (s *extensionService) start() error {
	table := db.GetTable[extensionData]()

	for _, factory := range allExtensionsMap {
		data, err := table.Get(factory.Id)
		if data == nil || err != nil {
			log.Warn("Data of Extension ", factory.Id, " not found, creating new one")
			data = &extensionData{Id: factory.Id, Enable: false}
			if err := table.UpdateInsert(data); err != nil {
				log.Warn("Failed to create new extension data: ", err, " ", factory.Id)
				return err
			}
		}

		if data.Enable {
			if err := loadExtension(factory); err != nil {
				return fmt.Errorf("failed to load extension %s: %w", data.Id, err)
			}
		}
	}

	return nil
}

func (s *extensionService) Close() error {
	for _, extension := range enabledExtensionsMap {
		if err := (*extension).Close(); err != nil {
			return err
		}
	}
	return nil
}

func (s *extensionService) Type() string { return "extension" }

func (s *extensionService) Tag() string { return "extension-service" }

func init() {
	service_manager.Register(&extensionService{})
}
